package lint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

const (
	modeDiagnose = "diagnose"
	modeFix      = "fix"
)

func New() clix.Subcommand {
	return clix.NewLeaf("lint", "Go の lint checker を実行する", Run)
}

func Run(ctx context.Context, in clix.RunInput) error {
	return doRun(ctx, in.Args(), in.Stdout(), in.Stderr(), &execOps{})
}

// ops は外部副作用 (golangci-lint / go mod tidy) を test 内で fake に差し替える fault boundary (FLM_APP_0009 §mock を採用しない / fake を採用する)。
type ops interface {
	runLint(ctx context.Context, moduleRoot string, args []string, stdout, stderr io.Writer) error
	runTidyApply(ctx context.Context, moduleRoot string) (output []byte, err error)
	runTidyDiff(ctx context.Context, moduleRoot string, stderr io.Writer) (diff string, err error)
}

type execOps struct{}

func (*execOps) runLint(ctx context.Context, moduleRoot string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "golangci-lint", args...) //nolint:gosec // G204: caller (本 endpoint) が固定 binary 名 + 内部組立 argv を渡す経路で外部入力ではない。
	cmd.Dir = moduleRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// caller (runLintForTarget) は err の identity を見ず非 nil → 違反検出として扱うのみのため wrap は不要。
	return ex.Wrap(cmd.Run())
}

func (*execOps) runTidyApply(ctx context.Context, moduleRoot string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	return out, ex.Wrap(err)
}

func (*execOps) runTidyDiff(ctx context.Context, moduleRoot string, stderr io.Writer) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy", "-diff")
	cmd.Dir = moduleRoot
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	// `go mod tidy -diff` は drift があるとき exit 1 で diff を stdout に出すため、 ExitError は drift 検出として扱い caller 側で stdout 内容を見て判定させる (caller が "" のときは tidy 状態と判定)。
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.String(), nil
	}
	return stdout.String(), err
}

func doRun(ctx context.Context, args []string, stdout, stderr io.Writer, op ops) error {
	mode, ok := resolveMode(os.Getenv("FLAME_CHECKER_MODE"))
	if !ok {
		fmt.Fprintf(stderr, "error: invalid FLAME_CHECKER_MODE='%s' (expected 'fix' or 'diagnose')\n", os.Getenv("FLAME_CHECKER_MODE"))
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}

	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: flame check go lint <package_dir>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}

	failed := false
	// seen modules を slice + set で保持し、 入力順を保ったまま重複排除する (出力の決定論性確保 / shell 版 declare -A の置換)。
	var moduleOrder []string
	seenModules := map[string]struct{}{}

	for _, pkgTarget := range args {
		if !runLintForTarget(ctx, op, pkgTarget, mode, stdout, stderr, &moduleOrder, seenModules) {
			failed = true
		}
	}

	for _, moduleRoot := range moduleOrder {
		if !runTidyForModule(ctx, op, moduleRoot, repoRoot, mode, stderr) {
			failed = true
		}
	}

	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

func resolveMode(raw string) (string, bool) {
	switch raw {
	case "":
		return modeDiagnose, true
	case modeFix, modeDiagnose:
		return raw, true
	default:
		return "", false
	}
}

func runLintForTarget(
	ctx context.Context,
	op ops,
	pkgTarget string,
	mode string,
	stdout, stderr io.Writer,
	moduleOrder *[]string,
	seenModules map[string]struct{},
) bool {
	info, err := os.Stat(pkgTarget)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "FAIL: %s: not a directory\n", pkgTarget)
		return false
	}
	pkgAbs, err := filepath.Abs(pkgTarget)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: not a directory\n", pkgTarget)
		return false
	}
	moduleRoot, ok := findModuleRoot(pkgAbs)
	if !ok {
		fmt.Fprintf(stderr, "FAIL: %s: 親方向に go.mod が見つからない\n", pkgTarget)
		return false
	}

	pkgPath := relativeModulePath(moduleRoot, pkgAbs)

	if _, dup := seenModules[moduleRoot]; !dup {
		seenModules[moduleRoot] = struct{}{}
		*moduleOrder = append(*moduleOrder, moduleRoot)
	}

	lintArgs := []string{"run"}
	if mode == modeFix {
		lintArgs = append(lintArgs, "--fix")
	}
	lintArgs = append(lintArgs, pkgPath)

	if err := op.runLint(ctx, moduleRoot, lintArgs, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: golangci-lint %s が違反を検出した\n", pkgTarget, pkgPath)
		return false
	}
	return true
}

func runTidyForModule(
	ctx context.Context,
	op ops,
	moduleRoot string,
	repoRoot string,
	mode string,
	stderr io.Writer,
) bool {
	relModule := relPathOrAbs(repoRoot, moduleRoot)
	ok := true

	if mode == modeFix {
		applyOut, err := op.runTidyApply(ctx, moduleRoot)
		if err != nil {
			fmt.Fprintf(stderr, "FAIL: %s: go mod tidy が失敗した:\n", relModule)
			if len(applyOut) > 0 {
				fmt.Fprint(stderr, string(applyOut))
				if !bytes.HasSuffix(applyOut, []byte("\n")) {
					fmt.Fprintln(stderr)
				}
			}
			ok = false
		}
	}

	// stderr は `go: downloading ...` 等の進捗メッセージを含み、 これを diff と誤判定すると platform 限定の transitive dep (例: cobra → mousetrap (Windows のみ)) が初回 fetch される度に tidy 違反扱いになるため、 stdout のみを diff として capture する (stderr は そのまま CI ログに流す)。
	tidyDiff, runErr := op.runTidyDiff(ctx, moduleRoot, stderr)
	if runErr != nil || tidyDiff != "" {
		fmt.Fprintf(stderr, "FAIL: %s: go.mod / go.sum が tidy 状態でない (go mod tidy で整える):\n", relModule)
		if tidyDiff != "" {
			fmt.Fprint(stderr, tidyDiff)
			if !strings.HasSuffix(tidyDiff, "\n") {
				fmt.Fprintln(stderr)
			}
		}
		ok = false
	}
	return ok
}

func findModuleRoot(dir string) (string, bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func relativeModulePath(moduleRoot, pkgAbs string) string {
	rel, err := filepath.Rel(moduleRoot, pkgAbs)
	if err != nil || rel == "." {
		return "./"
	}
	return "./" + filepath.ToSlash(rel)
}

func relPathOrAbs(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}
