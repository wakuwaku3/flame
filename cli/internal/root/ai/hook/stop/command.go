package stop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wakuwaku3/flame/cli/internal/check/bucketize"
	"github.com/wakuwaku3/flame/cli/internal/check/dispatch"
	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

// agentMarkerRe は shell 版 `grep -qE '"agent_(id|type)"[[:space:]]*:'` と一致させる。 Stop hook は main agent でのみ起動する想定だが、 `claude --agent <name>` 起動時の input には agent_id / agent_type が含まれるため、 その場合は保険として skip する。
var agentMarkerRe = regexp.MustCompile(`"agent_(id|type)"\s*:`)

// stopHookActiveRe は shell 版 `grep -qE '"stop_hook_active"[[:space:]]*:[[:space:]]*true'` と一致させる。 Stop hook が `decision: "block"` を返すと Claude は継続するが、 静的検査違反を AI が解消できない場合 (修正が新たな違反を生む / 修正対象が AI の操作範囲外 等) は同じ block が永続発火し無限ループになる。 Claude Code が継続中であることを示す stop_hook_active=true を検出した場合は本処理を skip しループを断つ (Claude Code 公式が想定するガード経路)。
var stopHookActiveRe = regexp.MustCompile(`"stop_hook_active"\s*:\s*true`)

const (
	stateRelDir   = ".claude/.cache/stop-hook"
	stateFileName = "state.tsv"
	lockFileName  = ".lock"
	lockTimeout   = 60 * time.Second
	lockPollEvery = 50 * time.Millisecond
	checkerMode   = "fix"
)

// checkFn は changed file 群を静的検査に流す責務を表す関数型。 production は runStaticCheck を渡し、 test は predetermined 出力を返す fake を inject する (FLM_APP_0009 §mock を採用しない / fake を採用)。
type checkFn = func(ctx context.Context, repo string, files []string) (output string, exitCode int, err error)

func New() clix.Subcommand {
	return clix.NewLeaf("stop", "Stop hook (AI ターン終端) を実行する", run)
}

func run(ctx context.Context, in clix.RunInput) error {
	return doRun(ctx, in, runStaticCheck)
}

func doRun(ctx context.Context, in clix.RunInput, runCheck checkFn) error {
	input, err := io.ReadAll(in.Stdin())
	if err != nil {
		return ex.Wrap(err)
	}
	if agentMarkerRe.Match(input) {
		return nil
	}
	if stopHookActiveRe.Match(input) {
		return nil
	}

	repo := os.Getenv("CLAUDE_PROJECT_DIR")
	if repo == "" {
		repo = "."
	}
	stateDir := filepath.Join(repo, stateRelDir)
	if mkErr := os.MkdirAll(stateDir, fsperm.Dir); mkErr != nil { //nolint:gosec // G703: stateDir は CLAUDE_PROJECT_DIR (= repo root) 配下の固定 path で外部入力ではない。
		return ex.Wrap(mkErr)
	}
	lockPath := filepath.Join(stateDir, lockFileName)
	statePath := filepath.Join(stateDir, stateFileName)

	lockFile, lockOK, lockErr := acquireLock(ctx, lockPath)
	if lockErr != nil {
		return ex.Wrap(lockErr)
	}
	if !lockOK {
		// ロック取れなくても本処理を妨げない (非ブロック扱い、 shell 版と同じ exit 0)。
		fmt.Fprintln(in.Stderr(), "stop-hook: failed to acquire lock within 60s")
		return nil
	}
	defer releaseLock(lockFile)

	currentHead := gitHeadOrPlaceholder(ctx, repo)
	files := collectExistingFiles(ctx, repo)

	stateExists, lastHash, stateErr := loadState(statePath, currentHead)
	if stateErr != nil {
		return ex.Wrap(stateErr)
	}

	currentHash := make(map[string]string, len(files))
	changed := make([]string, 0, len(files))
	for _, f := range files {
		h := gitHashObject(ctx, repo, f)
		currentHash[f] = h
		if prev, ok := lastHash[f]; !ok || prev != h {
			changed = append(changed, f)
		}
	}

	// state 不在の初回起動では「AI がこのターンで変更したファイル」と「ユーザ WIP として元々 working tree にあるファイル」を区別できない。 両者を「未検査」扱いで全件静的検査に流すと、 ユーザ WIP の既存違反まで AI に修正させてしまう。 初回は現状を承認済み baseline として記録し、 以降の差分のみ静的検査に流す。
	if !stateExists {
		return ex.Wrap(commitState(statePath, currentHead, currentHash))
	}
	if len(changed) == 0 {
		return ex.Wrap(commitState(statePath, currentHead, currentHash))
	}

	staticOutput, staticStatus, runErr := runCheck(ctx, repo, changed)
	if runErr != nil {
		return ex.Wrap(runErr)
	}
	if staticStatus != 0 {
		if emitErr := emitBlockReason(in.Stdout(), staticOutput); emitErr != nil {
			return ex.Wrap(emitErr)
		}
		return nil
	}

	// fix モードで checker がファイルを書き換えた場合、 changed[] に含まれるファイルの実際の hash が collect 時 (上の current_hash) と異なる可能性がある。 そのまま commit_state すると fix 前の hash を baseline に書き、 次回 hook fire で「変更あり」と誤検知して再 invoke が走る (no-op だが不要な往復)。 fix 後の hash で current_hash を上書きしてから state 確定。
	for _, f := range changed {
		if fileExists(filepath.Join(repo, f)) {
			currentHash[f] = gitHashObject(ctx, repo, f)
		}
	}
	return ex.Wrap(commitState(statePath, currentHead, currentHash))
}

// gitHeadOrPlaceholder は shell 版 `git rev-parse HEAD 2>/dev/null || echo "NO_HEAD"` 相当。 fresh repo で HEAD 不在の場合に baseline 失効として扱える文字列を返す。
func gitHeadOrPlaceholder(ctx context.Context, repo string) string {
	out, err := runGit(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return "NO_HEAD"
	}
	return strings.TrimSpace(out)
}

// collectExistingFiles は modified + untracked の合算 dedup 後、 working tree に実在する path のみを返す。 shell 版 `git diff --name-only --diff-filter=ACMR HEAD` + `git ls-files --others --exclude-standard` + dedup + 実在 filter と挙動を揃える (shell 版が `|| true` 経路で git エラーを握り潰すため、 本関数も失敗時は空 list 返却で素通り)。
func collectExistingFiles(ctx context.Context, repo string) []string {
	modified := gitListLines(ctx, repo, "diff", "--name-only", "--diff-filter=ACMR", "HEAD")
	untracked := gitListLines(ctx, repo, "ls-files", "--others", "--exclude-standard")
	merged := dedupNonEmpty(append(modified, untracked...))
	existing := make([]string, 0, len(merged))
	for _, f := range merged {
		if fileExists(filepath.Join(repo, f)) {
			existing = append(existing, f)
		}
	}
	return existing
}

func gitListLines(ctx context.Context, repo string, args ...string) []string {
	out, err := runGit(ctx, repo, args...)
	if err != nil {
		return nil
	}
	lines := strings.Split(out, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	return cleaned
}

func dedupNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// gitHashObject は shell 版 `git hash-object "$f" 2>/dev/null || echo "missing"` 相当。 失敗時は "missing" を返して、 baseline 比較で「変更あり」扱いに帰着させる。
func gitHashObject(ctx context.Context, repo, file string) string {
	out, err := runGit(ctx, repo, "hash-object", file)
	if err != nil {
		return "missing"
	}
	return strings.TrimSpace(out)
}

func runGit(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: git argv は stop hook 内部の固定文字列または stop hook が `git ls-files` / `git diff` から得た repo 内 path のみで、 外部入力ではない。
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", err //nolint:wrapcheck // 当 helper の error は呼び出し側で挙動分岐 (shell 版の `|| echo "..."` 相当の fallback) のみに使うため、 stack 付与は呼び出し側 (gitHeadOrPlaceholder 等) に閉じる。
	}
	return string(out), nil
}

// loadState は state.tsv を読み、 (state 存在フラグ, 前回 hash, error) を返す。 state は 1 行目に HEAD コミット SHA、 2 行目以降に <path>\t<hash>。 HEAD が変われば baseline 失効として全 changed 扱い (commit/checkout/pull/rebase で発生)。
func loadState(path, currentHead string) (exists bool, hashes map[string]string, err error) { //nolint:nonamedreturns // gocritic unnamedResult のための named return。 戻り値 3 つの意味を caller 側で読みやすくする目的。
	f, err := os.Open(path) //nolint:gosec // G304: stop hook が baseline 永続化用に組み立てた固定 path (state_dir/state.tsv) で、 caller 制御下の任意値ではない。
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil, nil
		}
		return false, nil, ex.Wrap(err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return true, map[string]string{}, nil
	}
	recordedHead := scanner.Text()
	hashes = map[string]string{}
	if recordedHead != currentHead {
		return true, hashes, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		before, after, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		hashes[before] = after
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return true, hashes, ex.Wrap(scanErr)
	}
	return true, hashes, nil
}

// commitState は state.tsv を atomic に書き戻す。 現在の HEAD を baseline として完全に上書きすることで、 過去の HEAD のエントリは自動的に消える (shell 版の挙動と同じ)。 中途で fail した場合は tmp file を残さず削除する (= 並行起動 / 次回起動で stale tmp が残らない invariant を保つ)。
func commitState(path, head string, hashes map[string]string) error {
	tmp := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsperm.File) //nolint:gosec // G304: tmp path は state.tsv に隣接する固定 path で、 外部入力ではない。
	if err != nil {
		return ex.Wrap(err)
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp) //nolint:gosec // G703: 同上、 tmp は内部組立 path。
	}
	w := bufio.NewWriter(f)
	if _, wErr := fmt.Fprintln(w, head); wErr != nil {
		cleanup()
		return ex.Wrap(wErr)
	}
	// shell 版は連想配列キーの iteration 順が未定義だが、 Go 側は決定論的に sort 済 list で書き出す (test の安定化目的)。 state は loadState が key 単位 lookup するため順序非依存。
	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, wErr := fmt.Fprintf(w, "%s\t%s\n", k, hashes[k]); wErr != nil {
			cleanup()
			return ex.Wrap(wErr)
		}
	}
	if flushErr := w.Flush(); flushErr != nil {
		cleanup()
		return ex.Wrap(flushErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(tmp) //nolint:gosec // G703: 同上。
		return ex.Wrap(closeErr)
	}
	if renameErr := os.Rename(tmp, path); renameErr != nil { //nolint:gosec // G703: tmp / path は同 caller 内で構築した固定 path で外部入力ではない。
		_ = os.Remove(tmp) //nolint:gosec // G703: 同上。
		return ex.Wrap(renameErr)
	}
	return nil
}

// runStaticCheck は changed ファイルを bucketize で content type 別に振り分け、 dispatch 経由で in-process Go ネイティブ checker を並列起動し、 (合算出力, exit code, error) を返す (FLM_FEA_0001 / FLM_FEA_0004: cli から bash subprocess を起動しない)。 FLAME_CHECKER_MODE=fix は検査対象 checker (golangci-lint --fix 等) が自動適用 mode に切り替わる起動契約で、 本 endpoint の責務範囲では fix 適用を必ず要求する (= 違反検出時に修正前 hash を baseline にしないため)。
func runStaticCheck(ctx context.Context, repo string, files []string) (output string, exitCode int, err error) { //nolint:nonamedreturns // unnamedResult: 戻り値 3 つの意味を named return で明示。
	entries, bErr := bucketize.Bucketize(repo, files)
	if bErr != nil {
		return "", -1, ex.Wrap(bErr)
	}
	if len(entries) == 0 {
		return "", 0, nil
	}
	// FLAME_CHECKER_MODE は os.Getenv 経由で個別 checker (例: cli/internal/root/check/golang/lint) が読む process global 入力。 in-process 並列実行中も同 process 内のため、 dispatcher 起動の前後で setenv / unsetenv して fix mode を限定的に注入する (= ターン終端 hook 専用の挙動を他経路に染み出させない)。
	const envKey = "FLAME_CHECKER_MODE"
	old, hadOld := os.LookupEnv(envKey)
	if setErr := os.Setenv(envKey, checkerMode); setErr != nil {
		return "", -1, ex.Wrap(setErr)
	}
	defer func() {
		if hadOld {
			_ = os.Setenv(envKey, old)
		} else {
			_ = os.Unsetenv(envKey)
		}
	}()
	out, code, dErr := dispatch.Dispatch(ctx, entries, 0)
	if dErr != nil {
		return out, -1, ex.Wrap(dErr)
	}
	return out, code, nil
}

// emitBlockReason は shell 版 `jq -Rs '{decision: "block", reason: .}'` と同じ JSON を stdout に書き出す。 jq の default は 2-space indent / key 後ろ space / 末尾 newline 1 個 / key 順は insertion 順 (= decision, reason 固定) のため、 struct + json.MarshalIndent で再現する (map は Go の encoding/json が key を sort するため不可)。 reason は 「ターン終端の静的検査で違反を検出した。 以下を全て修正すること。\n\n<output>」 形式。
func emitBlockReason(stdout io.Writer, staticOutput string) error {
	reason := "ターン終端の静的検査で違反を検出した。 以下を全て修正すること。\n\n" + staticOutput
	payload := struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{
		Decision: "block",
		Reason:   reason,
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&payload); err != nil {
		return ex.Wrap(err)
	}
	_, _ = io.WriteString(stdout, buf.String())
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path) //nolint:gosec // G703: caller (collectExistingFiles) が repo 内 path を join した値で、 外部入力ではない。
	return err == nil && !info.IsDir()
}
