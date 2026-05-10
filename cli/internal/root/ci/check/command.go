package check

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ci/check/detect"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/check/summary"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("check",
		clix.WithCommandShort("CI check 補助"),
	))
	cmd.AddSubcommand(detect.New())
	cmd.AddSubcommand(summary.New())
	return cmd
}
