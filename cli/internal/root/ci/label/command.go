package label

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ci/label/path_base"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("label",
		clix.WithCommandShort("CI label 補助"),
	))
	cmd.AddSubcommand(path_base.New())
	return cmd
}
