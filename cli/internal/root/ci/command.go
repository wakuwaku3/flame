package ci

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ci/check"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/configure_go_auth"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/deploy"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/label"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/noop"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/release"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("ci",
		clix.WithCommandShort("CI 補助コマンド"),
	))
	cmd.AddSubcommand(check.New())
	cmd.AddSubcommand(configure_go_auth.New())
	cmd.AddSubcommand(deploy.New())
	cmd.AddSubcommand(release.New())
	cmd.AddSubcommand(label.New())
	cmd.AddSubcommand(noop.New())
	return cmd
}
