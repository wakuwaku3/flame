package release

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ci/release/lib"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/release/spec"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/release/tool"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("release",
		clix.WithCommandShort("CI release 補助"),
	))
	cmd.AddSubcommand(tool.New())
	cmd.AddSubcommand(lib.New())
	cmd.AddSubcommand(spec.New())
	return cmd
}
