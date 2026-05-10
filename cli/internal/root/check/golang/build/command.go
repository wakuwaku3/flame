package build

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
	return clix.NewLeaf("build", "Go の build checker を実行する", Run)
}

func Run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check go build <main_package_dir>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	failed := false
	for _, target := range args {
		if reason, ok := buildPackage(ctx, target); !ok {
			fmt.Fprintf(in.Stderr(), "FAIL: %s: %s\n", target, reason)
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// buildPackage は target dir の main package を build し、 失敗時は (FAIL 行用の理由文字列, false) を返す。 go toolchain の stderr は service-level test で stderr 全体を exact match できるよう io.Discard に流す (shell 版は go の stderr を pipe しないため flame stderr に直接漏れていたが、 失敗 / 成功の判定情報としては FAIL 行と exit code で十分)。
func buildPackage(ctx context.Context, target string) (string, bool) {
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return "not a directory", false
	}
	pkgAbs, err := filepath.Abs(target)
	if err != nil {
		return err.Error(), false
	}
	moduleRoot, ok := findModuleRoot(pkgAbs)
	if !ok {
		return "親方向に go.mod が見つからない", false
	}
	pkgPath := buildPackagePath(moduleRoot, pkgAbs)
	// 検査用なのでバイナリ出力は破棄する (shell 版と同じ `-o /dev/null`)。
	// 起動コマンドは固定文字列 "go"、 引数の pkgPath は module root 配下の相対パスとして組み立てた値で外部入力ではない。 gosec G204 は変数引数の検出のみで意図性まで判定できないため、 当該行に限り false positive として局所抑制する (FLM_GEN_0006 §局所抑制が真に避けられない場合のみ、 理由を併記して例外的に許す)。 path-based グローバル無効化 (= 同 path 内の他の G204 検出も失う) ではなく当該 1 行に絞る。
	cmd := exec.CommandContext(ctx, "go", "build", "-o", os.DevNull, pkgPath) //nolint:gosec // G204: flame CLI 内部で組み立てた package path による go toolchain 起動
	cmd.Dir = moduleRoot
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if runErr := cmd.Run(); runErr != nil {
		return fmt.Sprintf("go build %s が失敗した", pkgPath), false
	}
	return "", true
}

func findModuleRoot(dir string) (string, bool) {
	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

// buildPackagePath は module root 直下の package 指定では `./` を、 サブディレクトリでは `./<rel>` を返す。
func buildPackagePath(moduleRoot, pkgAbs string) string {
	rel, err := filepath.Rel(moduleRoot, pkgAbs)
	if err != nil || rel == "." {
		return "./"
	}
	return "./" + filepath.ToSlash(rel)
}
