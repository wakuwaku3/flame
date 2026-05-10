package test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

func New() clix.Subcommand {
	return clix.NewLeaf("test", "Go の test checker を実行する", Run)
}

func Run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check go test <test_package_dir>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	failed := false
	for _, pkgTarget := range args {
		if !runGoTest(ctx, pkgTarget, in.Stdout(), in.Stderr()) {
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

func runGoTest(ctx context.Context, pkgTarget string, stdout, stderr io.Writer) bool {
	info, err := os.Stat(pkgTarget)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "FAIL: %s: not a directory\n", pkgTarget)
		return false
	}
	pkgAbs, err := filepath.Abs(pkgTarget)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: %s\n", pkgTarget, err)
		return false
	}
	moduleRoot, ok := findModuleRoot(pkgAbs)
	if !ok {
		fmt.Fprintf(stderr, "FAIL: %s: 親方向に go.mod が見つからない\n", pkgTarget)
		return false
	}
	pkgPath := goPackagePath(moduleRoot, pkgAbs)
	// 起動コマンドは固定文字列 "go"、 引数の pkgPath は module root からの相対 path で flame CLI 内で組み立てた値。 gosec G204 は変数引数の検出のみで意図性まで判定できないため、 当該行に限り false positive として局所抑制する (FLM_GEN_0006 §局所抑制が真に避けられない場合のみ、 理由を併記して例外的に許す)。 path-based グローバル無効化 (= 同 path 内の他の G204 検出も失う) ではなく当該 1 行に絞る。
	cmd := exec.CommandContext(ctx, "go", "test", pkgPath) //nolint:gosec // G204: flame CLI 内で組み立てた相対 path による subprocess 起動
	cmd.Dir = moduleRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: go test %s が違反を検出した\n", pkgTarget, pkgPath)
		return false
	}
	return true
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

func goPackagePath(moduleRoot, pkgAbs string) string {
	rel, err := filepath.Rel(moduleRoot, pkgAbs)
	if err != nil || rel == "." {
		return "./"
	}
	return "./" + filepath.ToSlash(rel)
}
