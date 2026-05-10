package install

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wakuwaku3/flame/lib/ex"
)

// vendorGitignoreBlock は flame.gitignore に追加する vendor 領域の ignore ブロック (FLM_FEA_0003 §vendor の git 追跡)。 親 dir 全体を ignore すると Git が下に降りないため、 *  で直接子のみを ignore してから !vendor/flame で flame だけ unignore する。
const vendorGitignoreBlock = "# flame harness は vendor/flame/ 配下を SoT として追跡する (FLM_FEA_0003)。\n# 親 dir 全体を ignore すると Git が下に降りないため、 *  で直接子のみを\n# ignore してから !vendor/flame で flame だけ unignore する。\nvendor/*\n!vendor/flame\n"

// applyGitignore は repo root の .gitignore に vendor/* と !vendor/flame を冪等追記する。 既に行が含まれていれば no-op。 manifest.ignore に `.gitignore` が含まれていれば no-op (caller 側で skip 判定済を想定)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期的な local file IO のみで cancel 経路を持たない。
func applyGitignore(_ context.Context, repoRoot string) error {
	path := filepath.Join(repoRoot, ".gitignore")
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ex.Wrapf(os.WriteFile(path, []byte(vendorGitignoreBlock), filePerm), "write .gitignore scaffold: %s", path)
	case err != nil:
		return ex.Wrapf(err, "read .gitignore: %s", path)
	}
	if hasVendorIgnoreBlock(existing) {
		return nil
	}
	updated := appendGitignoreBlock(existing)
	if err := os.WriteFile(path, updated, filePerm); err != nil {
		return ex.Wrapf(err, "write .gitignore: %s", path)
	}
	return nil
}

func hasVendorIgnoreBlock(content []byte) bool {
	s := string(content)
	return strings.Contains(s, "vendor/*") && strings.Contains(s, "!vendor/flame")
}

func appendGitignoreBlock(existing []byte) []byte {
	out := append([]byte(nil), existing...)
	if len(out) > 0 && !strings.HasSuffix(string(out), "\n") {
		out = append(out, '\n')
	}
	if len(out) > 0 {
		out = append(out, '\n')
	}
	out = append(out, []byte(vendorGitignoreBlock)...)
	return out
}
