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
	if _, err := os.Stat(filepath.Join(cwd, "flame.yaml")); err != nil {
		return ex.Errorf("flame.yaml not found in current directory: %s", cwd)
	}
	return Run(ctx, cwd, in.Stdout(), in.Stderr())
}
