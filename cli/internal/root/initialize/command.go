// Package initialize は `flame init` subcommand を実装する ([FLI_FEA_0002](../../../../docs/adr/feature/FLI_FEA_0002__flame_cli.md) §flame init による flame.yaml の初期生成)。 利用側 repository が flame harness を導入する初手として cwd 直下に flame.yaml を生成する。 npm init / cargo init と同じ慣習で source / version を順に prompt する対話モード + `-y` flag で skip + `--source` / `--version` flag で個別 override の 3 モード。 package 名は Go 言語仕様の `init()` 関数命名と衝突しないよう `initialize` とする (`flame devbox init` 同様の例外措置)。
package initialize

import (
	"context"
	"os"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	flagYes     = "yes"
	flagSource  = "source"
	flagVersion = "version"
)

// New は親 package (cli/internal/root) から binary version を引数で受け取って leaf を組み立てる。 root.Version は当該 package 内に閉じるため、 子 package である本 package が root を import すると循環依存になる。 initialize.New(root.Version) の形で値だけ渡す経路を取る (FLM_APP_0008 §subcommand package の階層 §直接の親 package 以外から子 package を import しない)。
func New(binaryVersion string) clix.Subcommand {
	return clix.NewCommand(clix.NewCommandConfig("init",
		clix.WithCommandShort("flame.yaml を cwd に初期生成する"),
		clix.WithCommandBoolFlag(flagYes, "y", false, "対話を skip して全 default で生成する (npm init -y 相当)"),
		clix.WithCommandStringFlag(flagSource, "", "", "flame.source の default を override する (空文字なら default 採用 / 対話 prompt)"),
		clix.WithCommandStringFlag(flagVersion, "", "", "flame.version の default を override する (空文字なら default 採用 / 対話 prompt)"),
		clix.WithCommandRunE(func(ctx context.Context, in clix.RunInput) error {
			return runLeaf(ctx, in, binaryVersion)
		}),
	))
}

func runLeaf(ctx context.Context, in clix.RunInput, binaryVersion string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	return Run(ctx, &RunOptions{
		RepoRoot:        cwd,
		BinaryVersion:   binaryVersion,
		Yes:             in.BoolFlag(flagYes),
		SourceOverride:  in.StringFlag(flagSource),
		VersionOverride: in.StringFlag(flagVersion),
		Stdin:           in.Stdin(),
		Stdout:          in.Stdout(),
		Stderr:          in.Stderr(),
	})
}
