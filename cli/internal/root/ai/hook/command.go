package hook

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ai/hook/pre_push"
	"github.com/wakuwaku3/flame/cli/internal/root/ai/hook/stop"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("hook",
		clix.WithCommandShort("Claude Code hook entrypoint"),
	))
	cmd.AddSubcommand(pre_push.New())
	cmd.AddSubcommand(stop.New())
	return cmd
}
