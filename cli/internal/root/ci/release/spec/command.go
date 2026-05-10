package spec

import (
	"github.com/wakuwaku3/flame/cli/internal/root/ci/release/spec/lib"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("spec",
		clix.WithCommandShort("Release spec emission"),
	))
	cmd.AddSubcommand(lib.New())
	return cmd
}
