package shell

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
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
	// scanBufferMax は bufio.Scanner の 1 行あたり読み込み上限 (1 MiB)。 shell スクリプトの行長は実用上 1 MiB を超えないため十分な余裕として設定する。
	scanBufferMax = 1024 * 1024
)

// allowlist は慣習的に大文字を使う POSIX / bash 規定環境変数を local 変数として扱える例外集合 (FLM_APP_0002 §変数の命名)。
// workflowTestFilenamePattern は FLM_APP_0002 §配置・命名 の例外規定: `.github/workflows/tests/<basename>.sh` は対応 workflow (FLM_ENG_0003) の basename (snake_case + `__` 区切り) を継承する。 各 segment は github_actions checker の `segInner` (= `[a-z][a-z0-9]*(?:_[a-z0-9]+)*`) と同形を取り、 `(trg|wf)__<seg>(__<seg>)?` を許す。
var (
	filenamePattern             = regexp.MustCompile(`^[a-z][a-z0-9-]*\.sh$`)
	workflowTestFilenamePattern = regexp.MustCompile(`^(trg|wf)__[a-z][a-z0-9]*(?:_[a-z0-9]+)*(?:__[a-z][a-z0-9]*(?:_[a-z0-9]+)*)?\.sh$`)
	allowlistPattern            = regexp.MustCompile(
		`^(IFS|OLDIFS|LANG|LC_ALL|LC_TIME|LC_NUMERIC|LC_COLLATE|LC_MONETARY|LC_MESSAGES|LC_CTYPE|PATH|PS1|PS2|PS3|PS4|REPLY|TMPDIR)$`,
	)
	upperVarLinePattern = regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]+)=`)
	exportPrefixPattern = regexp.MustCompile(`^\s*(export|local|declare|readonly|typeset)\s`)
)

func New() clix.Subcommand {
	return clix.NewLeaf("shell", "shell スクリプトを検査する", Run)
}

func Run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check shell <shell_file>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	failed := false
	for _, file := range args {
		if !isValidFilename(file) {
			fmt.Fprintf(in.Stderr(), "FAIL: %s: %s (got: %s)\n", file, filenameViolationMessage(file), filepath.Base(file))
			failed = true
		}
	}
	shellcheckFailed, shellcheckErr := runShellcheck(ctx, cwd, args, in.Stdout(), in.Stderr())
	if shellcheckErr != nil {
		return shellcheckErr
	}
	if shellcheckFailed {
		failed = true
	}
	for _, file := range args {
		violations, scanErr := scanUpperSnakeLocals(file)
		if scanErr != nil {
			fmt.Fprintf(in.Stderr(), "FAIL: %s: %s\n", file, scanErr)
			failed = true
			continue
		}
		for _, v := range violations {
			fmt.Fprintf(in.Stderr(), "FAIL: %s:%d: '%s' is UPPER_SNAKE_CASE; non-exported locals must be lower_snake_case (FLM_APP_0002)\n", file, v.lineNo, v.name)
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// isValidFilename は FLM_APP_0002 §配置・命名 を実装する: 基本は kebab-case
// だが、 `.github/workflows/tests/` 直下に置く workflow 本体に対応する test
// script は対応 workflow basename (snake_case + `__` 区切り) を継承する。
// `.github/workflows/tests/shared/` のように subdir 配下のファイルは例外
// 対象外で kebab-case に従う。
func isValidFilename(path string) bool {
	base := filepath.Base(path)
	if isWorkflowTestScriptPath(path) && workflowTestFilenamePattern.MatchString(base) {
		return true
	}
	return filenamePattern.MatchString(base)
}

// isWorkflowTestScriptPath は path の親 dir が `.github/workflows/tests` 直下
// (= shared/ などの subdir ではない) であるかを判定する。 例外規定の対象
// dir 制限を、 path の祖先 3 階層が `.github/workflows/tests` の順で一致するか
// で判定する。
func isWorkflowTestScriptPath(path string) bool {
	dir := filepath.ToSlash(filepath.Dir(path))
	if !strings.HasSuffix(dir, ".github/workflows/tests") && dir != ".github/workflows/tests" {
		return false
	}
	return true
}

// filenameViolationMessage は path に応じて FAIL message を切り替える。
// `.github/workflows/tests/` 直下のファイルは「kebab-case か workflow basename
// (snake_case + `__` 区切り)」 を許容する仕様のため、 単純な kebab-case 強制
// メッセージだと利用者が workflow basename への rename を逆に解除してしまう。
func filenameViolationMessage(path string) string {
	if isWorkflowTestScriptPath(path) {
		return "filename must be kebab-case or inherit corresponding workflow basename (snake_case + '__' separators) with .sh extension (FLM_APP_0002)"
	}
	return "filename must be kebab-case with .sh extension"
}

// `-x` は `# shellcheck source=...` directive を follow し、 sourced ファイルが入力に含まれない場合の SC1091 (info) を回避するため。
// `--source-path` は flame CLI が repo root cwd で起動される前提 (FLM_FEA_0004) を維持するため。
// shellcheck の non-zero exit (= 違反検出) は (true, nil) で返し、 子プロセス起動失敗 (バイナリ不在等) のみ error にする。
func runShellcheck(ctx context.Context, cwd string, files []string, stdout, stderr io.Writer) (bool, error) {
	cmdArgs := append([]string{"-x", "--source-path=" + cwd}, files...)
	// shellcheck は固定外部バイナリで、 cmdArgs は flame CLI 起動時 argv 直系。 G204 は変数引数の検出のみで意図性まで判定できないため、 当該 1 行に限り false positive として局所抑制する (FLM_GEN_0006)。
	cmd := exec.CommandContext(ctx, "shellcheck", cmdArgs...) //nolint:gosec // G204: flame CLI 内部固定の外部バイナリ + argv 由来引数の起動
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

type upperVarViolation struct {
	name   string
	lineNo int
}

// 行頭から UPPER_SNAKE_CASE= で始まる変数のうち、 export / local / declare / readonly / typeset 接頭辞が無く、
// allowlist (POSIX / bash 規定環境変数) にも該当しないものを違反として収集する (FLM_APP_0002 §変数の命名)。
func scanUpperSnakeLocals(path string) ([]upperVarViolation, error) {
	f, err := os.Open(path) //nolint:gosec // G304: 検査対象 path は flame check shell の argv 直系で、 endpoint の責務として外部入力を読み込む。
	if err != nil {
		return nil, ex.Wrap(err)
	}
	defer func() { _ = f.Close() }()
	var violations []upperVarViolation
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, scanBufferMax)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		match := upperVarLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if exportPrefixPattern.MatchString(line) {
			continue
		}
		name := match[1]
		if allowlistPattern.MatchString(name) {
			continue
		}
		violations = append(violations, upperVarViolation{lineNo: lineNo, name: name})
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, ex.Wrap(scanErr)
	}
	return violations, nil
}
