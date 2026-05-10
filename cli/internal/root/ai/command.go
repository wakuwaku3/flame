package ai

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ai/hook"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("ai",
		clix.WithCommandShort("AI 駆動の補助コマンド"),
	))
	cmd.AddSubcommand(hook.New())
	return cmd
}
