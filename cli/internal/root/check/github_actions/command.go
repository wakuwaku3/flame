package github_actions

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
	// inline `run:` block の上限。 FLM_ENG_0003 §inline shell の制限 で 3 行 / 300 字を超える場合は flame CLI subcommand へ抽出する規約のため、 違反検出のしきい値を定数化する。
	inlineRunMaxLines = 3
	inlineRunMaxChars = 300
	// FLM_APP_0001 §自然言語: heuristic として 15 文字以上の `#` コメントで非 ASCII を 1 文字も含まないものを English-prose 候補と扱う。 短いコメント (< 15 chars) は誤判定リスクが高いため通す。
	commentMinChars = 15
	// scanBufferMax は bufio.Scanner の 1 行あたり読み込み上限 (1 MiB)。 workflow YAML の行長は実用上 1 MiB を超えないため十分な余裕として設定する。
	scanBufferMax = 1024 * 1024
	// yaml.v3 の MappingNode.Content は key / value が 2 要素で 1 entry を成す表現。 mapping の entry 数は `len(Content) / mappingPair` で算出する。
	mappingPair = 2
	// asciiMax は 7-bit ASCII 上限 (= 0x7F)。 1 文字も非 ASCII を含まない行を「日本語 / 全角文字を 1 文字も含まない」 と見なすため (FLM_APP_0001 §自然言語 heuristic)。
	asciiMax = 0x7F
)

// FLM_ENG_0003 §トリガー層 で許可する top-level / job-level key 集合。 値は固定なので package level の sort 済 slice に固める (毎回 sort 不要、 表示時にも JSON-array literal として安定整形される)。
var (
	allowedTrgTopKeys = []string{"jobs", "name", "on"}
	allowedTrgJobKeys = []string{"name", "permissions", "secrets", "uses", "with"}
	// FLM_ENG_0003 §トリガー層 で明示的に禁止される job-level key。 検出時は許可キー違反とは別の FAIL 行を出してメッセージ上で禁止性を可視化する。
	forbiddenTrgJobKeys = []string{"steps", "run", "runs-on", "needs"}
)

// 2025 snapshot で GitHub Actions が解釈する event 名集合。 `pull_request_comment` は GHA に存在しない (`issue_comment` が PR/Issue 双方を発火する) ので含めない。
var knownEvents = []string{
	"branch_protection_rule", "check_run", "check_suite", "create", "delete",
	"deployment", "deployment_status", "discussion", "discussion_comment", "fork",
	"gollum", "issue_comment", "issues", "label", "merge_group", "milestone", "page_build",
	"project", "project_card", "project_column", "public", "pull_request",
	"pull_request_review", "pull_request_review_comment", "pull_request_target",
	"push", "registry_package", "release", "repository_dispatch", "repository_ruleset",
	"schedule", "status", "watch", "workflow_call", "workflow_dispatch", "workflow_run",
}

// `branches:` / `tags:` filter を持つ event 集合。 discriminator が `all` または filter 値そのものに対応するか検査するため (FLM_ENG_0003)。
var branchFilterEvents = map[string]bool{
	"push":                true,
	"pull_request":        true,
	"pull_request_target": true,
}

// `types:` filter (activity types) を持つ event 集合。 discriminator が types を `_` 連結したものと一致するかを検査するため (FLM_ENG_0003)。
var activityTypeEvents = map[string]bool{
	"branch_protection_rule":      true,
	"check_run":                   true,
	"check_suite":                 true,
	"discussion":                  true,
	"discussion_comment":          true,
	"issue_comment":               true,
	"issues":                      true,
	"label":                       true,
	"merge_group":                 true,
	"milestone":                   true,
	"project":                     true,
	"project_card":                true,
	"project_column":              true,
	"pull_request":                true,
	"pull_request_review":         true,
	"pull_request_review_comment": true,
	"pull_request_target":         true,
	"registry_package":            true,
	"release":                     true,
	"watch":                       true,
	"workflow_run":                true,
}

// `trg__` / `wf__` の `__` 区切りを構文的に一意にするため、 segment 内に連続 `_` を許さない (FLM_ENG_0003 §命名)。
const segInner = `[a-z][a-z0-9]*(?:_[a-z0-9]+)*`

var (
	trgFilenamePattern  = regexp.MustCompile(`^(?:flame-)?trg__(` + segInner + `)__(` + segInner + `)\.yaml$`)
	wfFilenamePattern   = regexp.MustCompile(`^wf__` + segInner + `(?:__` + segInner + `)?\.yaml$`)
	segInnerOnlyPattern = regexp.MustCompile(`^` + segInner + `$`)
	// 行頭または空白直後の `#` をコメントとして検出する。 YAML 文字列リテラル中 (`"foo#bar"` 等) の `#` を誤検出しないため位置制約を入れる。 captured group [1] が `#` の前文字 (空文字 or 空白)、 [2] が `#` 後の本文。
	commentLinePattern = regexp.MustCompile(`(^|\s)#(.*)$`)
	// path 様 token のうち letter / digit / 区切り記号 (`.`, `_`, `@`, `/`, `-`) のみで構成されるもの。
	pathTokenPattern = regexp.MustCompile(`^[a-zA-Z0-9._@/-]+$`)
	// 純 letter token (`This`, `comment` 等) は自然言語語彙と見なす。 path 様判定の例外ケース。
	letterOnlyPattern = regexp.MustCompile(`^[a-zA-Z]+$`)
	// `>>` の後ろに `$GITHUB_OUTPUT` (引用符あり/なし) が続く形を step output 書き込みとして検出する (FLM_ENG_0003 §step 出力の可観測性)。
	githubOutputRedirectPattern = regexp.MustCompile(`>>\s*"?\$GITHUB_OUTPUT"?`)
	// 同一行内で `tee -a` を経由していれば書き込みと console 出力が同時化されているとみなす。
	teeAppendPattern = regexp.MustCompile(`tee\s+-a`)
)

func New() clix.Subcommand {
	return clix.NewLeaf("github-actions", "GitHub Actions ワークフローを検査する", Run)
}

func Run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check github-actions <workflow_file>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}

	yamlInputs, badExtInputs := splitByExtension(args)
	failed := false
	for _, f := range badExtInputs {
		fmt.Fprintf(in.Stderr(), "FAIL: %s: GitHub Actions workflow must use the '.yaml' extension; FLM_ENG_0003 standardizes on '.yaml' (found other suffix)\n", f)
		failed = true
	}

	if len(yamlInputs) == 0 {
		if failed {
			return ex.Wrap(clix.NewExitError(exitCodeFailure))
		}
		return nil
	}

	if actionlintFailed, err := runActionlint(ctx, yamlInputs, in.Stdout(), in.Stderr()); err != nil {
		return err
	} else if actionlintFailed {
		failed = true
	}

	for _, path := range yamlInputs {
		if violations := checkWorkflow(path); len(violations) > 0 {
			for _, v := range violations {
				fmt.Fprintf(in.Stderr(), "FAIL: %s\n", v)
			}
			failed = true
		}

		testFailed, err := runWorkflowTest(ctx, path, in.Stdout(), in.Stderr())
		if err != nil {
			return err
		}
		if testFailed {
			failed = true
		}
	}

	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// FLM_ENG_0003 §test の配置規約に従い workflow path から対応 test script の path を導出する。 通常 (`wf__*` / `trg__*`) は同階層 `<dir>/tests/<stem>.sh` を返すが、 `flame-` prefix を持つ install copy (FLM_FEA_0003 §workflow の install 命名規約) は vendor SoT 側 `<repo_root>/vendor/flame/.github/workflows/tests/<stem-without-flame->.sh` に test を置く規約のためそちらを返す。 配置先を caller に委ねず関数内に閉じ込めることで、 lint と test の起動経路が同じ path 解決ロジックを共有することを保証する。
func testScriptPath(workflowPath string) string {
	dir := filepath.Dir(workflowPath)
	base := filepath.Base(workflowPath)
	stem := strings.TrimSuffix(base, ".yaml")
	if rest, isInstallCopy := strings.CutPrefix(stem, "flame-"); isInstallCopy {
		repoRoot := filepath.Dir(filepath.Dir(dir))
		return filepath.Join(repoRoot, "vendor", "flame", ".github", "workflows", "tests", rest+".sh")
	}
	return filepath.Join(dir, "tests", stem+".sh")
}

// 不在 (= os.ErrNotExist) を「lint 違反」 として扱い、 不在以外の os.Stat エラー (権限不足等) は system エラーとして error 経路に分岐させるのは、 FLM_ENG_0003 §test が「対応 test script を必ず置く」 を lint 段の検出責務に固定しているため。 子プロセスの exit-non-zero を (true, nil) で返すのは runActionlint と同じ扱い (lint / test の集積側で failed フラグに合算する)。
func runWorkflowTest(ctx context.Context, workflowPath string, stdout, stderr io.Writer) (bool, error) {
	scriptPath := testScriptPath(workflowPath)
	info, statErr := os.Stat(scriptPath)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			fmt.Fprintf(stderr, "FAIL: %s: missing corresponding test script at %s (FLM_ENG_0003 §test)\n", workflowPath, scriptPath)
			return true, nil
		}
		return false, ex.Wrap(statErr)
	}
	if info.IsDir() {
		fmt.Fprintf(stderr, "FAIL: %s: expected test script at %s but found a directory (FLM_ENG_0003 §test)\n", workflowPath, scriptPath)
		return true, nil
	}
	// bash は固定の system shell。 scriptPath は flame CLI 起動時 argv 由来 path から決定論的に派生 (testScriptPath) しており、 検査対象 .github/workflows/tests/<stem>.sh を起動するのが endpoint の責務。
	cmd := exec.CommandContext(ctx, "bash", scriptPath) //nolint:gosec // G204: flame CLI 内部固定の bash + caller 制御下の argv 由来 path の起動
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if runErr == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return true, nil
	}
	return false, ex.Wrap(runErr)
}

// detect.sh は `.yaml` / `.yml` 双方を当該 checker に流す (FLM_ENG_0003 §拡張子)。 `.yml` を accepted bucket に混ぜず separately FAIL 化することで、 actionlint / カスタム検査が `.yml` を silently 容認することを防ぐ。
func splitByExtension(args []string) (yamlInputs, badExt []string) {
	for _, f := range args {
		if strings.HasSuffix(f, ".yaml") {
			yamlInputs = append(yamlInputs, f)
		} else {
			badExt = append(badExt, f)
		}
	}
	return yamlInputs, badExt
}

func runActionlint(ctx context.Context, files []string, stdout, stderr io.Writer) (bool, error) {
	cmdArgs := append([]string{"--"}, files...)
	// actionlint は固定外部バイナリで、 cmdArgs は flame CLI 起動時 argv 直系。 gosec G204 は変数引数の検出のみで意図性まで判定できないため、 当該 1 行に限り false positive として局所抑制する (FLM_GEN_0006 §局所抑制が真に避けられない場合のみ)。
	cmd := exec.CommandContext(ctx, "actionlint", cmdArgs...) //nolint:gosec // G204: flame CLI 内部固定の外部バイナリ + argv 由来引数の起動
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if runErr == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return true, nil
	}
	return false, ex.Wrap(runErr)
}

func checkWorkflow(path string) []string {
	base := filepath.Base(path)
	unprefixed := strings.TrimPrefix(base, "flame-")
	if !strings.HasPrefix(unprefixed, "trg__") && !strings.HasPrefix(unprefixed, "wf__") {
		return []string{path + ": filename must start with 'trg__', 'wf__', or 'flame-trg__' (install copy) (FLM_ENG_0003 / FLM_FEA_0003)"}
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: flame check github-actions が受け取る path は CLI 起動時 argv そのもので、 検査対象として外部入力を読み込むのが endpoint の責務。
	if err != nil {
		return []string{fmt.Sprintf("%s: failed to read file: %s", path, err)}
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []string{fmt.Sprintf("%s: failed to parse YAML: %s", path, err)}
	}

	var failures []string
	failures = append(failures, checkUniversalRules(path, &doc)...)
	failures = append(failures, checkCommentLanguage(path, data)...)
	failures = append(failures, checkGithubOutputTee(path, data)...)

	if strings.HasPrefix(unprefixed, "trg__") {
		failures = append(failures, checkTrg(path, &doc)...)
	} else {
		failures = append(failures, checkWf(path, &doc)...)
	}
	return failures
}

func rootMapping(doc *yaml.Node) (*yaml.Node, bool) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, false
	}
	return root, true
}

func mappingGet(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}

// mappingKeys は YAML mapping の key を sort 済 slice として返す。 検出順依存をなくし、 出力表記を決定論的に保つ。
func mappingKeys(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(node.Content)/mappingPair)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}
	sort.Strings(keys)
	return keys
}

func checkUniversalRules(path string, doc *yaml.Node) []string {
	root, ok := rootMapping(doc)
	if !ok {
		return []string{path + ": top-level YAML must be a mapping (FLM_ENG_0003)"}
	}
	base := filepath.Base(path)
	fileStem := strings.TrimSuffix(strings.TrimPrefix(base, "flame-"), ".yaml")
	expectedPrefix := fileStem + " / "

	failures := checkFetchDepth(path, root, "")
	failures = append(failures, checkInlineRunBlocks(path, root)...)
	failures = append(failures, checkJobNames(path, root, expectedPrefix)...)
	return failures
}

func checkFetchDepth(filePath string, node *yaml.Node, dotted string) []string {
	var failures []string
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := node.Content[i+1]
			childPath := dotted + "." + key
			if key == "fetch-depth" && value.Kind == yaml.ScalarNode && value.Value == "0" {
				failures = append(failures, fmt.Sprintf("%s: 'fetch-depth: 0' is forbidden at %s (FLM_ENG_0003 §最小 clone — use ref: refs/pull/<n>/head or a fixed head_sha and fetch base SHA individually)", filePath, childPath))
			}
			failures = append(failures, checkFetchDepth(filePath, value, childPath)...)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			failures = append(failures, checkFetchDepth(filePath, child, fmt.Sprintf("%s[%d]", dotted, i))...)
		}
	case yaml.DocumentNode:
		for _, child := range node.Content {
			failures = append(failures, checkFetchDepth(filePath, child, dotted)...)
		}
	case yaml.ScalarNode, yaml.AliasNode:
	}
	return failures
}

func checkInlineRunBlocks(filePath string, root *yaml.Node) []string {
	jobs, ok := mappingGet(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode {
		return nil
	}
	var failures []string
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		jid := jobs.Content[i].Value
		job := jobs.Content[i+1]
		steps, ok := mappingGet(job, "steps")
		if !ok || steps.Kind != yaml.SequenceNode {
			continue
		}
		for sidx, step := range steps.Content {
			runNode, ok := mappingGet(step, "run")
			if !ok || runNode.Kind != yaml.ScalarNode {
				continue
			}
			text := runNode.Value
			// 末尾改行の有無に依らず行数を一意に数えるため、 末尾改行を一旦剥がしてから区切る。
			trimmed := strings.TrimRight(text, "\n")
			lines := 0
			if trimmed != "" {
				lines = strings.Count(trimmed, "\n") + 1
			}
			chars := len(text)
			if lines > inlineRunMaxLines || chars > inlineRunMaxChars {
				failures = append(failures, fmt.Sprintf("%s: inline 'run:' block at .jobs.%s.steps[%d] exceeds limits (lines=%d, chars=%d; max %d lines / %d chars) — extract to a flame CLI subcommand (FLM_ENG_0003 §inline shell の制限 / FLM_FEA_0005)", filePath, jid, sidx, lines, chars, inlineRunMaxLines, inlineRunMaxChars))
			}
		}
	}
	return failures
}

// checkJobNames は unnamed / mismatched を別行で報告する。 検査結果の grep / 目視確認のため出力表記を決定論的に保つ。
func checkJobNames(filePath string, root *yaml.Node, expectedPrefix string) []string {
	jobs, ok := mappingGet(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode {
		return nil
	}
	var unnamed []string
	var mismatched []jobNameEntry
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		jid := jobs.Content[i].Value
		job := jobs.Content[i+1]
		nameNode, ok := mappingGet(job, "name")
		if !ok {
			unnamed = append(unnamed, jid)
			continue
		}
		if nameNode.Kind != yaml.ScalarNode || !strings.HasPrefix(nameNode.Value, expectedPrefix) {
			mismatched = append(mismatched, jobNameEntry{Key: jid, Name: nameNode.Value})
		}
	}
	var failures []string
	if len(unnamed) > 0 {
		failures = append(failures, fmt.Sprintf("%s: every job must declare 'name:' (FLM_ENG_0003 §ジョブの命名); missing on %s", filePath, formatStringList(unnamed)))
	}
	if len(mismatched) > 0 {
		failures = append(failures, fmt.Sprintf("%s: every job 'name:' must start with '%s' (FLM_ENG_0003 §ジョブの命名); offenders %s", filePath, expectedPrefix, formatJobNameList(mismatched)))
	}
	return failures
}

type jobNameEntry struct {
	Key  string
	Name string
}

// formatStringList は string slice を `["a","b"]` 形式の compact JSON literal で整形する。 検査結果の grep / 目視確認のため決定論的な表記を維持する。
func formatStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = `"` + s + `"`
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// formatJobNameList は job ID と name の対を `[{"key":"a","name":"b"},...]` 形式の compact JSON literal で整形する。
func formatJobNameList(entries []jobNameEntry) string {
	if len(entries) == 0 {
		return "[]"
	}
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = `{"key":"` + e.Key + `","name":"` + e.Name + `"}`
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// FLM_APP_0001 §自然言語 を heuristic として実装する。 静的検査では完全な日本語判定が困難なため、 短い (< commentMinChars) ものや lint directive を許容する近似判定とする。
func checkCommentLanguage(filePath string, data []byte) []string {
	var failures []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(nil, scanBufferMax)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		match := commentLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		content := strings.TrimPrefix(match[2], " ")
		if len(content) < commentMinChars {
			continue
		}
		if containsNonASCII(content) {
			continue
		}
		if strings.Contains(content, "yamllint") || strings.Contains(content, "actionlint") {
			continue
		}
		if isAllPathLike(content) {
			continue
		}
		preview := content
		const previewLimit = 60
		if len(preview) > previewLimit {
			preview = preview[:previewLimit]
		}
		failures = append(failures, fmt.Sprintf("%s:%d: English-prose comment detected (FLM_APP_0001 §自然言語); rewrite in Japanese: '%s'", filePath, lineNo, preview))
	}
	return failures
}

func containsNonASCII(s string) bool {
	for i := range len(s) {
		if s[i] > asciiMax {
			return true
		}
	}
	return false
}

// letter-only token (`This` / `comment` 等) を自然言語語彙として除外するため、 path-like の判定では数字 / 区切り記号を含むものだけを許容する。
func isAllPathLike(content string) bool {
	tokens := strings.Fields(content)
	if len(tokens) == 0 {
		return false
	}
	for _, tok := range tokens {
		if !pathTokenPattern.MatchString(tok) {
			return false
		}
		if letterOnlyPattern.MatchString(tok) {
			return false
		}
	}
	return true
}

func checkGithubOutputTee(filePath string, data []byte) []string {
	var failures []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(nil, scanBufferMax)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if !githubOutputRedirectPattern.MatchString(line) {
			continue
		}
		if teeAppendPattern.MatchString(line) {
			continue
		}
		failures = append(failures, fmt.Sprintf(`%s:%d: '>>"$GITHUB_OUTPUT"' without 'tee -a' is forbidden (FLM_ENG_0003 §step 出力の可観測性); pipe through 'tee -a' so the value is visible in the CI log too`, filePath, lineNo))
	}
	return failures
}

func checkTrg(path string, doc *yaml.Node) []string {
	base := filepath.Base(path)
	if !trgFilenamePattern.MatchString(base) {
		return []string{path + ": trg__ filename must match '[flame-]trg__<event>__<discriminator>.yaml' (FLM_ENG_0003 / FLM_FEA_0003)"}
	}
	stem := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(base, "flame-"), "trg__"), ".yaml")
	event, disc, ok := splitEventAndDisc(stem)
	if !ok {
		return []string{path + ": event segment is not a known GitHub Actions event name (FLM_ENG_0003)"}
	}
	if disc == "" || !segInnerOnlyPattern.MatchString(disc) {
		return []string{fmt.Sprintf("%s: discriminator '%s' must be lower snake_case starting with a letter (FLM_ENG_0003)", path, disc)}
	}

	root, ok := rootMapping(doc)
	if !ok {
		return []string{path + ": top-level YAML must be a mapping (FLM_ENG_0003)"}
	}

	var failures []string
	if extras := keysDiff(mappingKeys(root), allowedTrgTopKeys); len(extras) > 0 {
		failures = append(failures, fmt.Sprintf("%s: trg__ workflow may only contain top-level keys %s; found extras %s (FLM_ENG_0003)", path, formatStringList(allowedTrgTopKeys), formatStringList(extras)))
	}

	onNode, hasOn := mappingGet(root, "on")
	if !hasOn {
		return append(failures, path+": missing 'on' (FLM_ENG_0003)")
	}
	if onNode.Kind != yaml.MappingNode {
		return append(failures, fmt.Sprintf("%s: 'on' must be a mapping declaring exactly the single event '%s' matching the filename (FLM_ENG_0003)", path, event))
	}
	onKeys := mappingKeys(onNode)
	if len(onKeys) != 1 || onKeys[0] != event {
		return append(failures, fmt.Sprintf("%s: 'on' must declare exactly the single event '%s' matching the filename (FLM_ENG_0003)", path, event))
	}

	jobsNode, hasJobs := mappingGet(root, "jobs")
	if !hasJobs || jobsNode.Kind != yaml.MappingNode {
		return append(failures, path+": 'jobs' must contain exactly one entry (FLM_ENG_0003)")
	}
	jobCount := len(jobsNode.Content) / mappingPair
	if jobCount != 1 {
		return append(failures, path+": 'jobs' must contain exactly one entry (FLM_ENG_0003)")
	}

	jobNode := jobsNode.Content[1]
	jobKeys := mappingKeys(jobNode)
	jobExtras := keysDiff(jobKeys, allowedTrgJobKeys)
	if len(jobExtras) > 0 {
		forbiddenPresent := intersect(jobExtras, forbiddenTrgJobKeys)
		if len(forbiddenPresent) > 0 {
			failures = append(failures, fmt.Sprintf("%s: trg__ job must not contain %s (FLM_ENG_0003)", path, formatStringList(forbiddenPresent)))
		}
		failures = append(failures, fmt.Sprintf("%s: trg__ job may only contain %s; found extras %s (FLM_ENG_0003)", path, formatStringList(allowedTrgJobKeys), formatStringList(jobExtras)))
	}
	if _, hasUses := mappingGet(jobNode, "uses"); !hasUses {
		failures = append(failures, path+": trg__ job must dispatch via 'uses:' (FLM_ENG_0003)")
	}

	onEventNode, _ := mappingGet(onNode, event)
	failures = append(failures, checkTrgEventDiscriminator(path, event, disc, onEventNode)...)
	return failures
}

// schedule 等 activityTypeEvents / branchFilterEvents 双方の listing 外の event は discriminator を自由識別子として扱い検査を skip する。 FLM_ENG_0003 で意味論を明示していない event 型まで本静的検査が誤検出を出さないため。
func checkTrgEventDiscriminator(path, event, disc string, onEventNode *yaml.Node) []string {
	if activityTypeEvents[event] {
		if onEventNode == nil || onEventNode.Kind != yaml.MappingNode {
			return []string{fmt.Sprintf("%s: '%s' event must declare 'types: [...]' and the discriminator must equal the types joined by '_' (FLM_ENG_0003)", path, event)}
		}
		typesNode, hasTypes := mappingGet(onEventNode, "types")
		if !hasTypes || typesNode.Kind != yaml.SequenceNode {
			return []string{fmt.Sprintf("%s: '%s' event must declare 'types: [...]' and the discriminator must equal the types joined by '_' (FLM_ENG_0003)", path, event)}
		}
		types := scalarSequence(typesNode)
		expected := strings.Join(types, "_")
		if disc != expected {
			return []string{fmt.Sprintf("%s: discriminator '%s' must equal types joined by '_' = '%s' (FLM_ENG_0003)", path, disc, expected)}
		}
		return nil
	}
	if branchFilterEvents[event] {
		var branches, tags []string
		var hasBranches, hasTags bool
		if onEventNode != nil && onEventNode.Kind == yaml.MappingNode {
			if bNode, ok := mappingGet(onEventNode, "branches"); ok && bNode.Kind == yaml.SequenceNode {
				branches = scalarSequence(bNode)
				hasBranches = true
			}
			if tNode, ok := mappingGet(onEventNode, "tags"); ok && tNode.Kind == yaml.SequenceNode {
				tags = scalarSequence(tNode)
				hasTags = true
			}
		}
		if disc == "all" {
			// `all` は「両側フィルタなし」または「片側 `**` のみ」のいずれかに対応する (FLM_ENG_0003)。
			if !hasBranches && !hasTags {
				return nil
			}
			if hasBranches && !hasTags && len(branches) == 1 && branches[0] == "**" {
				return nil
			}
			if hasTags && !hasBranches && len(tags) == 1 && tags[0] == "**" {
				return nil
			}
			return []string{path + ": discriminator 'all' must correspond to no branches/tags filter, or branches: ['**'] with no tags, or tags: ['**'] with no branches (FLM_ENG_0003)"}
		}
		if hasBranches && len(branches) == 1 && branches[0] == disc {
			return nil
		}
		if hasTags && len(tags) == 1 && tags[0] == disc {
			return nil
		}
		return []string{fmt.Sprintf("%s: discriminator '%s' must match on.%s.branches or on.%s.tags as a single-element list (FLM_ENG_0003)", path, disc, event, event)}
	}
	return nil
}

// scalar 以外を空文字に置換するのは、 検査側 (caller) で意図的な mismatch として扱うため。
func scalarSequence(node *yaml.Node) []string {
	out := make([]string, 0, len(node.Content))
	for _, child := range node.Content {
		if child.Kind == yaml.ScalarNode {
			out = append(out, child.Value)
		} else {
			out = append(out, "")
		}
	}
	return out
}

// multi-word event (`pull_request_target` 等) と discriminator (`opened` 等) の境界は `__` のみだと曖昧なため、 longest-prefix で event 名を切り出す。
func splitEventAndDisc(stem string) (event, discriminator string, ok bool) {
	sorted := make([]string, len(knownEvents))
	copy(sorted, knownEvents)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	for _, ev := range sorted {
		prefix := ev + "__"
		if rest, found := strings.CutPrefix(stem, prefix); found {
			return ev, rest, true
		}
	}
	return "", "", false
}

func checkWf(path string, doc *yaml.Node) []string {
	base := filepath.Base(path)
	if !wfFilenamePattern.MatchString(base) {
		return []string{path + ": wf__ filename must match 'wf__<verb>[__<target>].yaml' (FLM_ENG_0003)"}
	}
	root, ok := rootMapping(doc)
	if !ok {
		return []string{path + ": top-level YAML must be a mapping (FLM_ENG_0003)"}
	}
	onNode, hasOn := mappingGet(root, "on")
	if !hasOn {
		return []string{path + ": missing 'on:' (FLM_ENG_0003)"}
	}
	if onNode.Kind != yaml.MappingNode {
		return []string{path + ": 'on:' must be a mapping (FLM_ENG_0003)"}
	}

	var failures []string
	wcNode, hasCall := mappingGet(onNode, "workflow_call")
	wdNode, hasDispatch := mappingGet(onNode, "workflow_dispatch")
	if !hasCall {
		failures = append(failures, path+": wf__ workflow must declare 'on.workflow_call' (FLM_ENG_0003)")
	}
	if !hasDispatch {
		failures = append(failures, path+": wf__ workflow must declare 'on.workflow_dispatch' (FLM_ENG_0003)")
	}

	wcInputs, wcInputsOk := optionalInputs(wcNode)
	wdInputs, wdInputsOk := optionalInputs(wdNode)
	if wcInputsOk != wdInputsOk {
		failures = append(failures, path+": workflow_call.inputs and workflow_dispatch.inputs must both be present or both absent (FLM_ENG_0003)")
		return failures
	}
	if !wcInputsOk {
		return failures
	}
	if wcInputs == nil || wdInputs == nil || wcInputs.Kind != yaml.MappingNode || wdInputs.Kind != yaml.MappingNode {
		failures = append(failures, path+": inputs must be a mapping under both workflow_call and workflow_dispatch (FLM_ENG_0003)")
		return failures
	}

	wcKeys := mappingKeys(wcInputs)
	wdKeys := mappingKeys(wdInputs)
	if !stringSliceEqual(wcKeys, wdKeys) {
		onlyCall := keysDiff(wcKeys, wdKeys)
		onlyDisp := keysDiff(wdKeys, wcKeys)
		failures = append(failures, fmt.Sprintf("%s: workflow_call.inputs and workflow_dispatch.inputs must declare the same input names; only in workflow_call=%s, only in workflow_dispatch=%s (FLM_ENG_0003)", path, formatStringList(onlyCall), formatStringList(onlyDisp)))
		return failures
	}

	for _, name := range wcKeys {
		wcEntry, _ := mappingGet(wcInputs, name)
		wdEntry, _ := mappingGet(wdInputs, name)
		sigC, okC := inputSignature(wcEntry)
		sigD, okD := inputSignature(wdEntry)
		if !okC || !okD {
			failures = append(failures, fmt.Sprintf("%s: input '%s' is malformed (each side must be a mapping with an explicit 'type:' key) (FLM_ENG_0003)", path, name))
		} else if sigC != sigD {
			failures = append(failures, fmt.Sprintf("%s: input '%s' has mismatched 'type' between workflow_call and workflow_dispatch (FLM_ENG_0003)", path, name))
		}
	}
	return failures
}

// optionalInputs は親が無い / null / inputs 未定義のすべてを (nil, false) で返す。 inputs が空 mapping の場合は (空 mapping ノード, true) を返す。
func optionalInputs(node *yaml.Node) (*yaml.Node, bool) {
	if node == nil {
		return nil, false
	}
	if node.Kind != yaml.MappingNode {
		return nil, false
	}
	inputs, ok := mappingGet(node, "inputs")
	if !ok {
		return nil, false
	}
	if inputs.Tag == "!!null" {
		return nil, false
	}
	return inputs, true
}

// inputSignature は entry が mapping でない / type フィールドが欠落 / type が scalar でない場合に (空文字, false) を返し caller 側で INVALID 扱いにさせる。
func inputSignature(entry *yaml.Node) (string, bool) {
	if entry == nil || entry.Kind != yaml.MappingNode {
		return "", false
	}
	typeNode, ok := mappingGet(entry, "type")
	if !ok || typeNode.Kind != yaml.ScalarNode {
		return "", false
	}
	return typeNode.Value, true
}

// keysDiff は a に含まれて b に含まれない key を sort 済 slice として返す (`a - b` 集合差)。
func keysDiff(a, b []string) []string {
	bset := make(map[string]struct{}, len(b))
	for _, x := range b {
		bset[x] = struct{}{}
	}
	var out []string
	for _, x := range a {
		if _, ok := bset[x]; !ok {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// intersect は a と b の両方に含まれる key を a の入力順を保って返す (禁止 key 検出経路で使用)。
func intersect(a, b []string) []string {
	bset := make(map[string]struct{}, len(b))
	for _, x := range b {
		bset[x] = struct{}{}
	}
	var out []string
	for _, x := range a {
		if _, ok := bset[x]; ok {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
