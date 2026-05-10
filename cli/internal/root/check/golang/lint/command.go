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

// usage / FLAME_CHECKER_MODE 違反は exit 2、 検査失敗は exit 1。
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
	mode, ok := resolveMode(os.Getenv("FLAME_CHECKER_MODE"))
	if !ok {
		fmt.Fprintf(in.Stderr(), "error: invalid FLAME_CHECKER_MODE='%s' (expected 'fix' or 'diagnose')\n", os.Getenv("FLAME_CHECKER_MODE"))
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}

	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check go lint <package_dir>...")
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
		if !runLintForTarget(ctx, in, pkgTarget, mode, &moduleOrder, seenModules) {
			failed = true
		}
	}

	for _, moduleRoot := range moduleOrder {
		if !runTidyForModule(ctx, in, moduleRoot, repoRoot, mode) {
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

// runLintForTarget は pkgTarget 1 件分の golangci-lint を実行し、 violation 無ければ true を返す。 module root 解決失敗 / dir 不在等の事前検証違反も同様に false を返して FAIL 行を残す。
func runLintForTarget(
	ctx context.Context,
	in clix.RunInput,
	pkgTarget string,
	mode string,
	moduleOrder *[]string,
	seenModules map[string]struct{},
) bool {
	info, err := os.Stat(pkgTarget)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(in.Stderr(), "FAIL: %s: not a directory\n", pkgTarget)
		return false
	}
	pkgAbs, err := filepath.Abs(pkgTarget)
	if err != nil {
		fmt.Fprintf(in.Stderr(), "FAIL: %s: not a directory\n", pkgTarget)
		return false
	}
	moduleRoot, ok := findModuleRoot(pkgAbs)
	if !ok {
		fmt.Fprintf(in.Stderr(), "FAIL: %s: 親方向に go.mod が見つからない\n", pkgTarget)
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

	cmd := exec.CommandContext(ctx, "golangci-lint", lintArgs...)
	cmd.Dir = moduleRoot
	cmd.Stdout = in.Stdout()
	cmd.Stderr = in.Stderr()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(in.Stderr(), "FAIL: %s: golangci-lint %s が違反を検出した\n", pkgTarget, pkgPath)
		return false
	}
	return true
}

// runTidyForModule は seen module 1 件分の go mod tidy 系検査を実行する。 fix mode では先に `go mod tidy` を適用し、 続いて diagnose / fix 共通で `go mod tidy -diff` を走らせて drift を検出する。
func runTidyForModule(
	ctx context.Context,
	in clix.RunInput,
	moduleRoot string,
	repoRoot string,
	mode string,
) bool {
	relModule := relPathOrAbs(repoRoot, moduleRoot)
	ok := true

	if mode == modeFix {
		applyCmd := exec.CommandContext(ctx, "go", "mod", "tidy")
		applyCmd.Dir = moduleRoot
		applyOut, err := applyCmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(in.Stderr(), "FAIL: %s: go mod tidy が失敗した:\n", relModule)
			if len(applyOut) > 0 {
				fmt.Fprint(in.Stderr(), string(applyOut))
				if !bytes.HasSuffix(applyOut, []byte("\n")) {
					fmt.Fprintln(in.Stderr())
				}
			}
			ok = false
		}
	}

	// stderr は `go: downloading ...` 等の進捗メッセージを含み、 これを diff と誤判定すると platform 限定の transitive dep (例: cobra → mousetrap (Windows のみ)) が初回 fetch される度に tidy 違反扱いになるため、 stdout のみを diff として capture する (stderr は そのまま CI ログに流す)。
	tidyDiff, runErr := runTidyDiff(ctx, moduleRoot, in.Stderr())
	if runErr != nil || tidyDiff != "" {
		fmt.Fprintf(in.Stderr(), "FAIL: %s: go.mod / go.sum が tidy 状態でない (go mod tidy で整える):\n", relModule)
		if tidyDiff != "" {
			fmt.Fprint(in.Stderr(), tidyDiff)
			if !strings.HasSuffix(tidyDiff, "\n") {
				fmt.Fprintln(in.Stderr())
			}
		}
		ok = false
	}
	return ok
}

func runTidyDiff(ctx context.Context, dir string, stderr io.Writer) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy", "-diff")
	cmd.Dir = dir
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
