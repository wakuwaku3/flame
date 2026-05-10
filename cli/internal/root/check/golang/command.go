package golang

import (
	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/build"
	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/lint"
	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/test"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("go",
		clix.WithCommandShort("Go-related checkers"),
	))
	cmd.AddSubcommand(build.New())
	cmd.AddSubcommand(lint.New())
	cmd.AddSubcommand(test.New())
	return cmd
}
