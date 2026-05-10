package deploy

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ci/deploy/detect"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/deploy/detect_lib"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/deploy/summary"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("deploy",
		clix.WithCommandShort("CI deploy 補助"),
	))
	cmd.AddSubcommand(detect.New())
	cmd.AddSubcommand(detect_lib.New())
	cmd.AddSubcommand(summary.New())
	return cmd
}
