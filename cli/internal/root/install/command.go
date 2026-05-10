package install

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

func New() clix.Subcommand {
	return clix.NewLeaf("install", "flame.yaml に基づき harness 資産を install / 更新する", run)
}

func run(ctx context.Context, in clix.RunInput) error {
	cwd, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	repoRoot, err := findRepoRoot(ctx, cwd)
	if err != nil {
		return err
	}
	return Run(ctx, repoRoot, in.Stdout(), in.Stderr())
}

// findRepoRoot は cwd から上方向に flame.yaml を探索する。 dev 経路 (`go run` from cli/) と install 後の経路 (任意の cwd から flame コマンドを叩く) の両方をカバーする。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func findRepoRoot(_ context.Context, start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "flame.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ex.Errorf("flame.yaml not found in %s or any ancestor", start)
		}
		dir = parent
	}
}
