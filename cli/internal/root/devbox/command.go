package devbox

import (
	"github.com/wakuwaku3/flame/cli/internal/root/devbox/initialize"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("devbox",
		clix.WithCommandShort("devbox 補助コマンド"),
	))
	cmd.AddSubcommand(initialize.New())
	return cmd
}
