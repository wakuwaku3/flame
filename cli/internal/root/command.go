// Package root は flame CLI の root command を組み立てる ([FLI_FEA_0002](../../../docs/adr/feature/FLI_FEA_0002__flame_cli.md))。 子 group / leaf は [FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md) §subcommand package の階層 に従い `<module>/internal/root/<cmd1>[/<cmd2>...]/` に物理ディレクトリで配置している。
package root

import (
	"context"

	"github.com/wakuwaku3/flame/cli/internal/root/ai"
	"github.com/wakuwaku3/flame/cli/internal/root/check"
	"github.com/wakuwaku3/flame/cli/internal/root/ci"
	"github.com/wakuwaku3/flame/cli/internal/root/devbox"
	"github.com/wakuwaku3/flame/cli/internal/root/initialize"
	rootinstall "github.com/wakuwaku3/flame/cli/internal/root/install"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

// Version は build 時に -ldflags `-X 'github.com/wakuwaku3/flame/cli/internal/root.Version=<semver>'` で上書きされる。
var Version = "0.0.0-dev"

func Execute(ctx context.Context, io clix.IO) error {
	r := clix.NewRoot(clix.NewRootConfig(
		"flame",
		Version,
		clix.WithRootShort("flame CLI"),
	))
	r.AddCommand(check.New())
	r.AddCommand(devbox.New())
	r.AddCommand(ai.New())
	r.AddCommand(ci.New())
	r.AddCommand(initialize.New(Version)) //nolint:contextcheck // contextcheck が root 直下 leaf の closure chain を「ctx 未伝搬」 と誤検出する false positive (install pkg と同じ理由)。 initialize pkg 内は runLeaf(ctx, in, ...) → Run(ctx, ...) で ctx を完全伝搬している (FLM_GEN_0006 §局所抑制が真に避けられない場合)。
	r.AddCommand(rootinstall.New())       //nolint:contextcheck // contextcheck が root 直下 leaf の closure chain を「ctx 未伝搬」 と誤検出する false positive。 install pkg 内は run(ctx, in) → Run(ctx, ...) → 全 IO 関数(ctx, ...) で ctx を完全伝搬しているが、 root 直下 leaf 特有のため group 経由 leaf (例: flame check go test) では発火しない (FLM_GEN_0006 §局所抑制が真に避けられない場合)。
	return ex.Wrap(r.Run(ctx, io))
}
