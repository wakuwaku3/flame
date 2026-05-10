// Package pre_push は Claude Code の PreToolUse hook (matcher: Bash) として呼ばれ、 git push 直前に AI レビューを起動する block を返す (FLM_ENG_0001 / FLI_FEA_0002)。 移行元 shell `scripts/pre-push-review.sh` と同一の挙動を Go ネイティブで再実装する。
package pre_push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

func New() clix.Subcommand {
	return clix.NewLeaf("pre-push", "PreToolUse hook (git push 直前) を実行する", run)
}

// hookInput は Claude Code が PreToolUse hook に流す JSON payload の最小構造。 .tool_name / .tool_input.command / .cwd 以外は本 hook で参照しない。
type hookInput struct {
	ToolName  string    `json:"tool_name"`
	ToolInput toolInput `json:"tool_input"`
	Cwd       string    `json:"cwd"`
}

type toolInput struct {
	Command string `json:"command"`
}

// blockDecision は Claude Code が PreToolUse hook の stdout JSON として解釈する block 命令 (`{"decision":"block","reason":"..."}`)。 shell 版 `jq -Rs '{decision:"block", reason: .}'` の出力形式と完全一致させる。
type blockDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// quotedRe は引用符内文字列を除去する regex 群。 shell 版 `sed -E "s/'[^']*'//g; s/\"([^\"\\\\]|\\\\.)*\"//g"` を Go regexp に翻訳する。 single quote は中身が改行を含まない最短一致、 double quote は escape 対応で最短一致。
var (
	singleQuotedRe = regexp.MustCompile(`'[^']*'`)
	doubleQuotedRe = regexp.MustCompile(`"([^"\\]|\\.)*"`)
)

// gitPushRe は引用符除去後文字列に対して `git push` 単体起動を検出する。 shell 版 `(^|[[:space:]]|&&|\|\||;|\|)[[:space:]]*git[[:space:]]+push([[:space:]]|$)` を Go regexp に翻訳する。 検査対象を ` $strip_quoted ` (= 前後にスペースを足した文字列) にしたうえで `(^|space|chain)` を判定する shell 側の挙動を、 Go では同様に空白でラップして判定する。
var gitPushRe = regexp.MustCompile(`(^|[\t ]|&&|\|\||;|\|)[\t ]*git[\t ]+push([\t ]|$)`)

// dryRunRe / deleteRe は git push の素通り条件 (--dry-run / --delete / -d)。 shell 版の判定式と同等。
var (
	dryRunRe = regexp.MustCompile(`[\t ]--dry-run([\t ]|=|$)`)
	deleteRe = regexp.MustCompile(`[\t ](--delete|-d)([\t ]|$)`)
)

// chainOpRe は他コマンドとの chain 検出用 (HEAD 不一致リスクで block する条件)。 shell 版 `&&|\|\||;|\||&[[:space:]]*$` と同等。
var chainOpRe = regexp.MustCompile(`&&|\|\||;|\||&[\t ]*$`)

// adrPathRe は to_review に ADR (`docs/adr/<category>/<file>.md`) が含まれるかの判定。 shell 版 bash `[[ "$f" =~ (^|/)docs/adr/[^/]+/[^/]+\.md$ ]]` と同等。
var adrPathRe = regexp.MustCompile(`(^|/)docs/adr/[^/]+/[^/]+\.md$`)

const (
	stateDirRel  = ".claude/.cache/pre-push-review"
	stateFileRel = "state.tsv"
)

// noUpstreamSentinel は upstream tracking が解決できない fallback (= origin/main) で current_upstream_sha が取れない場合のマーカー。 shell 版 `current_upstream_sha=$(git rev-parse "$upstream" 2>/dev/null || echo "NO_UPSTREAM")` と一致させる。
const noUpstreamSentinel = "NO_UPSTREAM"

func run(ctx context.Context, in clix.RunInput) error {
	payload, err := io.ReadAll(in.Stdin())
	if err != nil {
		return ex.Wrap(err)
	}

	var hi hookInput
	if len(payload) > 0 {
		if jsonErr := json.Unmarshal(payload, &hi); jsonErr != nil {
			return ex.Wrap(jsonErr)
		}
	}

	if hi.ToolName != "Bash" {
		return nil
	}

	cmd := hi.ToolInput.Command
	stripped := stripQuoted(cmd)
	wrapped := " " + stripped + " "

	if !gitPushRe.MatchString(wrapped) {
		return nil
	}
	if dryRunRe.MatchString(wrapped) {
		return nil
	}
	if deleteRe.MatchString(wrapped) {
		return nil
	}
	if chainOpRe.MatchString(stripped) {
		return emitBlock(in.Stdout(), chainBlockReason())
	}

	// hook input の cwd を一次経路、 CLAUDE_PROJECT_DIR を fallback (cwd 未提供な古い harness 互換) として、 git の認識する worktree root を確定する。 CLAUDE_PROJECT_DIR を直接 cd 起点にすると、 worktree から push しても main repo の HEAD を見て差分 0 と判定して素通りしてしまう (= AI レビュー空振り)。 git repo 外で偶然 hook が発火した場合は副作用なしで素通り。 state cache も rev-parse --show-toplevel 解決後の repo (= 当該 worktree root) 配下に置くことで worktree 別に分離される。
	hookCwd := hi.Cwd
	if hookCwd == "" {
		hookCwd = os.Getenv("CLAUDE_PROJECT_DIR")
	}
	if hookCwd == "" {
		hookCwd = "."
	}
	repoOut, repoErr := runGit(ctx, hookCwd, "rev-parse", "--show-toplevel")
	if repoErr != nil {
		return nil //nolint:nilerr // shell 版 PR #67 が `git -C "$hook_cwd" rev-parse --show-toplevel || exit 0` で「git repo 外なら副作用なしで素通り」 を実現しているのと挙動を揃えるため意図的に nil error を返す。
	}
	repo := strings.TrimSpace(repoOut)

	upstream, ok := resolveUpstream(ctx, repo)
	if !ok {
		return nil
	}

	files := gitDiffFiles(ctx, repo, upstream)
	if len(files) == 0 {
		return nil
	}

	stateDir := filepath.Join(repo, stateDirRel)
	if mkErr := os.MkdirAll(stateDir, fsperm.Dir); mkErr != nil { //nolint:gosec // G703: stateDir は git rev-parse --show-toplevel 由来 (= 当該 worktree root) 配下の固定 path で外部入力ではない。
		return ex.Wrap(mkErr)
	}
	stateFile := filepath.Join(stateDir, stateFileRel)

	currentUpstreamSHA := gitRevParse(ctx, repo, upstream)
	if currentUpstreamSHA == "" {
		currentUpstreamSHA = noUpstreamSentinel
	}

	reviewedHash := loadReviewedHash(stateFile, currentUpstreamSHA)
	currentHash := computeCurrentHash(ctx, repo, files)

	toReview := make([]string, 0, len(files))
	for _, f := range files {
		if reviewedHash[f] != currentHash[f] {
			toReview = append(toReview, f)
		}
	}
	if len(toReview) == 0 {
		return nil
	}

	if writeErr := writeState(stateFile, currentUpstreamSHA, currentHash, reviewedHash); writeErr != nil {
		return ex.Wrap(writeErr)
	}

	adrChanged := slices.ContainsFunc(toReview, adrPathRe.MatchString)
	extraAgents := loadStage1ExtraAgents(repo)

	reason := buildBlockReason(toReview, files, adrChanged, extraAgents)
	return emitBlock(in.Stdout(), reason)
}

// flameYamlRoot は `flame.yaml` 全体のうち本 hook が必要とする部分だけを抜き出す struct。 yaml.v3 の default は unknown field を許容するため、 他フィールド (`flame.harness.*` 等) が存在しても parse は失敗しない (FLM_FEA_0003 が `flame.yaml` schema を別途規定しているのと整合)。
type flameYamlRoot struct {
	Flame struct {
		AI struct {
			PrePush struct {
				Stage1ExtraAgents []string `yaml:"stage1_extra_agents"`
			} `yaml:"pre_push"`
		} `yaml:"ai"`
	} `yaml:"flame"`
}

// loadStage1ExtraAgents は `flame.yaml` から `flame.ai.pre_push.stage1_extra_agents` を読む。 file 不在 / parse error / フィールド欠如 はすべて soft fail として nil を返し、 hook の現行挙動と等価な経路に落とす (= shell 版の `|| true` 哲学に揃える)。 重複は先勝ち de-dup で正規化し、 yaml の記述順を保持する。
func loadStage1ExtraAgents(repo string) []string {
	data, err := os.ReadFile(filepath.Join(repo, "flame.yaml")) //nolint:gosec // G304: flame.yaml path は git rev-parse --show-toplevel 由来 (= 当該 worktree root) 配下の固定ファイル名で外部入力ではない。
	if err != nil {
		return nil
	}
	var root flameYamlRoot
	if uerr := yaml.Unmarshal(data, &root); uerr != nil {
		return nil
	}
	return dedup(root.Flame.AI.PrePush.Stage1ExtraAgents)
}

// dedup は items の重複を「先勝ち」で除く。 yaml 記述順を保ったまま重複だけを削るため map ベースの seen check と slice append を組合せる。
func dedup(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, s := range items {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// stripQuoted は shell 版の sed 2 段 (`s/'[^']*'//g; s/\"([^\"\\\\]|\\\\.)*\"//g`) と同じ順序・同じ意味で引用符内を空文字に置換する。 single quote → double quote の順序を維持することで、 `'...".."...'` のように single quote 内にある double quote を先に消費させる shell 挙動と一致させる。
func stripQuoted(cmd string) string {
	out := singleQuotedRe.ReplaceAllString(cmd, "")
	return doubleQuotedRe.ReplaceAllString(out, "")
}

// resolveUpstream は当該ブランチの upstream tracking を一次経路、 origin/main を fallback として返す (shell 版の 2 段判定と同じ)。 どちらも解決できない初期状態では (false) を返し、 caller は素通り (exit 0) する。
func resolveUpstream(ctx context.Context, repo string) (string, bool) {
	upstream, err := runGit(ctx, repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err == nil {
		upstream = strings.TrimSpace(upstream)
		if upstream != "" {
			return upstream, true
		}
	}
	if _, verifyErr := runGit(ctx, repo, "rev-parse", "--verify", "origin/main"); verifyErr == nil {
		return "origin/main", true
	}
	return "", false
}

// gitDiffFiles は upstream...HEAD の差分ファイル一覧を返す。 shell 版の `git diff "$upstream...HEAD" --name-only` と同等で、 失敗時は空 list (= 素通り扱い) を返す (shell 版の `|| true` 経路に揃える)。
func gitDiffFiles(ctx context.Context, repo, upstream string) []string {
	out, err := runGit(ctx, repo, "diff", upstream+"...HEAD", "--name-only")
	if err != nil {
		return nil
	}
	files := make([]string, 0)
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		files = append(files, line)
	}
	return files
}

// gitRevParse は失敗時に空文字を返す ($? を呼び出し側で見ないラッパ)。 shell 版 `git rev-parse "$upstream" 2>/dev/null || echo "NO_UPSTREAM"` の `||` 経路を caller 側で表現するため、 失敗判定だけを内側で吸収する。
func gitRevParse(ctx context.Context, repo, ref string) string {
	out, err := runGit(ctx, repo, "rev-parse", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func loadReviewedHash(stateFile, currentUpstreamSHA string) map[string]string {
	out := make(map[string]string)
	data, err := os.ReadFile(stateFile) //nolint:gosec // G304: state file path は git rev-parse --show-toplevel 由来 (= 当該 worktree root) 配下の固定相対 path (.claude/.cache/pre-push-review/state.tsv) で外部入力ではない (shell 版 `state_file` と同じ位置を読む経路)。
	if err != nil {
		return out
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return out
	}
	if strings.TrimRight(lines[0], "\r") != currentUpstreamSHA {
		return out
	}
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		path, hash, ok := strings.Cut(line, "\t")
		if !ok || path == "" {
			continue
		}
		out[path] = hash
	}
	return out
}

// computeCurrentHash は files 各要素の git hash-object を計算する。 missing file (= 削除) は "DELETED" マーカーで shell 版と一致させる。 hash 計算自体が失敗した場合は "missing" を入れる (shell 版 `git hash-object "$f" 2>/dev/null || echo "missing"` と同等)。
func computeCurrentHash(ctx context.Context, repo string, files []string) map[string]string {
	out := make(map[string]string, len(files))
	for _, f := range files {
		full := filepath.Join(repo, f)
		info, err := os.Stat(full) //nolint:gosec // G703: full は repo 内 path を join した値で、 git 出力由来 (= 外部入力ではない)。
		if err != nil || info.IsDir() {
			out[f] = "DELETED"
			continue
		}
		hash, hashErr := runGit(ctx, repo, "hash-object", f)
		if hashErr != nil {
			out[f] = "missing"
			continue
		}
		out[f] = strings.TrimSpace(hash)
	}
	return out
}

// writeState は state.tsv を atomic に rewrite する (1 行目: upstream sha、 2 行目以降: `path\thash` 行)。 currentHash と reviewedHash 由来 entry を merge し、 currentHash 側に存在しない reviewedHash entry のみ追加保持する (shell 版の 2 段ループと同じ意味)。 file system order に依存しないよう merge 後に key で sort する (shell の連想配列 iteration 順は非決定で、 Go の map iteration 順も非決定なので test 安定化のため sort 必須)。
func writeState(stateFile, currentUpstreamSHA string, currentHash, reviewedHash map[string]string) error {
	merged := make(map[string]string, len(currentHash)+len(reviewedHash))
	maps.Copy(merged, currentHash)
	for k, v := range reviewedHash {
		if _, ok := currentHash[k]; ok {
			continue
		}
		merged[k] = v
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	buf.WriteString(currentUpstreamSHA)
	buf.WriteByte('\n')
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte('\t')
		buf.WriteString(merged[k])
		buf.WriteByte('\n')
	}

	tmp := fmt.Sprintf("%s.tmp.%d", stateFile, os.Getpid())
	if err := os.WriteFile(tmp, []byte(buf.String()), fsperm.File); err != nil { //nolint:gosec // G703: tmp は stateFile に隣接する固定 path で、 caller 内部値。
		_ = os.Remove(tmp) //nolint:gosec // G703: 同上。 partial write 失敗時の tmp leak を避けるため明示削除。
		return ex.Wrap(err)
	}
	if err := os.Rename(tmp, stateFile); err != nil { //nolint:gosec // G703: 同上、 内部 path のみ。
		_ = os.Remove(tmp) //nolint:gosec // G703: 同上。 rename 失敗時の tmp leak を避けるため明示削除。
		return ex.Wrap(err)
	}
	return nil
}

// emitBlock は shell 版 `jq -Rs '{decision:"block", reason: .}'` と同じ JSON 表現 (= reason 末尾改行を保持、 末尾改行付き) を stdout に書く。 json.Encoder は SetIndent + SetEscapeHTML(false) で jq の default 整形 (2 スペース indent / `&` `<` `>` を escape しない) に揃える。 Encoder は Encode 呼び出しごとに末尾改行を自動付与するため、 caller 側で改行を足す必要は無い。
func emitBlock(stdout io.Writer, reason string) error {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return ex.Wrap(enc.Encode(&blockDecision{Decision: "block", Reason: reason}))
}

// chainBlockReason は shell 版 heredoc (chain 検出時) の本文と完全一致させる。 各行末に改行 (heredoc 最終行も改行付き)、 全体末尾も改行で締める ($'...\n' を `cat <<'EOS'` に流すと末尾改行が付く挙動を再現)。
func chainBlockReason() string {
	return `PreToolUse: git push が他コマンドと chain (&& / || / ; / | / & 等) されている。
hook 発火時点では先行コマンドが未実行で HEAD が push 直前状態と一致しないため、 push 対象差分を抽出できない。

修正方針:
- HEAD を変更する操作 (git commit / git pull --rebase / git rebase 等) と git push を別の Bash 呼び出しに分けて再実行する
- 例: 1 回目の Bash で ` + "`git commit -m '...'`" + `、 2 回目の Bash で ` + "`git push`" + `
`
}

// buildBlockReason は shell 版 heredoc (ADR 変更有無で 2 分岐) の本文を組み立てる。 file_list / all_file_list は shell の `printf '  - %s\n' "${...[@]}"` と同等で、 各 path に "  - " prefix と改行を付与する (printf は最終要素にも改行付与)。 extraAgents は repo-local 拡張として段階 1 並列起動 list に append する。
func buildBlockReason(toReview, files []string, adrChanged bool, extraAgents []string) string {
	fileList := formatBulletList(toReview)
	allFileList := formatBulletList(files)
	if adrChanged {
		return blockReasonWithADR(fileList, allFileList, extraAgents)
	}
	return blockReasonWithoutADR(fileList, allFileList, extraAgents)
}

// formatExtraAgentLines は repo-local 拡張 agent 名の slice を段階 1 並列起動 list に追記する形 (= 各 agent に default の「違反 fix」 経路文言を付ける) で整形する。 各行末 + 末尾に改行を 1 つだけ付け、 caller 側 format 文字列の `%s\n段階 2` 構造で正しく blank 行 1 つだけ確保される。 空 list の場合は空文字を返し、 既存の builtin reviewer 行と段階 2 の間の blank 行 1 つだけが残る。
func formatExtraAgentLines(extraAgents []string) string {
	if len(extraAgents) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range extraAgents {
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteString(": 返ってきた違反を全て fix する (新規 commit を積み上げる形で fix する。 既存 commit の amend / rebase は不要)")
		b.WriteByte('\n')
	}
	return b.String()
}

// formatBulletList は shell `printf '  - %s\n' "${arr[@]}"` 互換の文字列を返す。 空 list の場合 shell printf は何も出さないため空文字を返す。
func formatBulletList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, it := range items {
		b.WriteString("  - ")
		b.WriteString(it)
		b.WriteByte('\n')
	}
	return b.String()
}

func blockReasonWithADR(fileList, allFileList string, extraAgents []string) string {
	const builtinCount = 4
	totalCount := builtinCount + len(extraAgents)
	extraLines := formatExtraAgentLines(extraAgents)
	return fmt.Sprintf(`git push 直前の AI レビューを実行する。 push を試行する前に以下を実行すること。

段階 1 検査対象 (前回レビュー以降に変更されたファイルに限定):
%s
段階 2 検査対象 (push 対象差分の全ファイル):
%s
段階 1 (並列実行): 以下 %d つの subagent を Task ツールで並列起動し、 段階 1 検査対象を渡す。 各 subagent の戻り値の扱いは以下に従う。
- general-practices-reviewer: 返ってきた違反を全て fix する (新規 commit を積み上げる形で fix する。 既存 commit の amend / rebase は不要)
- rule-adr-sync-reviewer: 返ってきた違反を全て fix する (同上)
- test-coverage-reviewer: 返ってきた違反を全て fix する (同上)
- redundant-comment-remover: 本 subagent は違反指摘ではなく Edit / Write による削除を直接実行する。 親セッションは返却された削除レポートをそのまま受理し、 該当判定や残置判断のやり直しを行わない。 削除分は次回 commit に含めるのみ
%s
段階 2: Task ツールで adr-reviewer subagent を起動。 段階 1 検査対象 (= (a) 変更経路) と段階 2 検査対象 (= (b) ADR 整合性経路) の両リストを渡す。 違反を全て fix する (段階 1 と衝突時は ADR 優先。 fix は新規 commit を積み上げる)。 adr-reviewer には ADR §決定 に対する違反を「前回 review 以降の変更で発生したものではない」 「既存違反である」 等を理由に却下しないよう明示すること。

各 reviewer は 1 回だけ起動する。 fix 後は再度 git push を試行する (本 hook が再発火しても、 対象ファイル内容が同一なら素通りする)。
`, fileList, allFileList, totalCount, extraLines)
}

func blockReasonWithoutADR(fileList, allFileList string, extraAgents []string) string {
	const builtinCount = 3
	totalCount := builtinCount + len(extraAgents)
	extraLines := formatExtraAgentLines(extraAgents)
	return fmt.Sprintf(`git push 直前の AI レビューを実行する。 push を試行する前に以下を実行すること。

段階 1 検査対象 (前回レビュー以降に変更されたファイルに限定):
%s
段階 2 検査対象 (push 対象差分の全ファイル):
%s
段階 1 (並列実行): 以下 %d つの subagent を Task ツールで並列起動し、 段階 1 検査対象を渡す。 各 subagent の戻り値の扱いは以下に従う。
- general-practices-reviewer: 返ってきた違反を全て fix する (新規 commit を積み上げる形で fix する。 既存 commit の amend / rebase は不要)
- test-coverage-reviewer: 返ってきた違反を全て fix する (同上)
- redundant-comment-remover: 本 subagent は違反指摘ではなく Edit / Write による削除を直接実行する。 親セッションは返却された削除レポートをそのまま受理し、 該当判定や残置判断のやり直しを行わない。 削除分は次回 commit に含めるのみ
%s
段階 2: Task ツールで adr-reviewer subagent を起動。 段階 1 検査対象 (= (a) 変更経路) と段階 2 検査対象 (= (b) ADR 整合性経路) の両リストを渡す。 違反を全て fix する (段階 1 と衝突時は ADR 優先。 fix は新規 commit を積み上げる)。 adr-reviewer には ADR §決定 に対する違反を「前回 review 以降の変更で発生したものではない」 「既存違反である」 等を理由に却下しないよう明示すること。

各 reviewer は 1 回だけ起動する。 fix 後は再度 git push を試行する (本 hook が再発火しても、 対象ファイル内容が同一なら素通りする)。
`, fileList, allFileList, totalCount, extraLines)
}

// runGit は repo 配下で git を起動し stdout を返す。 子 process の stderr は捨てる (shell 版が `2>/dev/null` で抑制している経路と同じ)。 起動失敗 / non-zero 終了は err として返し、 caller が `|| true` 相当の素通り判断をする。
func runGit(ctx context.Context, repo string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: git argv は本 hook 内部で組み立てた固定値か git 出力由来の repo 内 path のみで、 外部入力ではない。
	c.Dir = repo
	c.Env = os.Environ()
	var stdout strings.Builder
	c.Stdout = &stdout
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), ex.Wrap(err)
		}
		return "", ex.Wrap(err)
	}
	return stdout.String(), nil
}
