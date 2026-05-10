// Package root は flame CLI の root command を組み立てる ([FLI_FEA_0002](../../../docs/adr/feature/FLI_FEA_0002__flame_cli.md))。 子 group / leaf は [FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md) §subcommand package の階層 に従い `<module>/internal/root/<cmd1>[/<cmd2>...]/` に物理ディレクトリで配置している。
package root

import (
	"context"

	"github.com/wakuwaku3/flame/cli/internal/root/ai"
	"github.com/wakuwaku3/flame/cli/internal/root/check"
	"github.com/wakuwaku3/flame/cli/internal/root/ci"
	"github.com/wakuwaku3/flame/cli/internal/root/devbox"
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
	return ex.Wrap(r.Run(ctx, io))
}
