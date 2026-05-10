package check

import (
	"github.com/wakuwaku3/flame/cli/internal/root/check/adr"
	"github.com/wakuwaku3/flame/cli/internal/root/check/devbox"
	"github.com/wakuwaku3/flame/cli/internal/root/check/document"
	"github.com/wakuwaku3/flame/cli/internal/root/check/flow_document"
	"github.com/wakuwaku3/flame/cli/internal/root/check/github_actions"
	"github.com/wakuwaku3/flame/cli/internal/root/check/golang"
	"github.com/wakuwaku3/flame/cli/internal/root/check/json"
	"github.com/wakuwaku3/flame/cli/internal/root/check/run"
	"github.com/wakuwaku3/flame/cli/internal/root/check/shell"
	"github.com/wakuwaku3/flame/cli/internal/root/check/yaml"
	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	cmd := clix.NewCommand(clix.NewCommandConfig("check",
		clix.WithCommandShort("Run static checkers"),
	))
	cmd.AddSubcommand(adr.New())
	cmd.AddSubcommand(devbox.New())
	cmd.AddSubcommand(document.New())
	cmd.AddSubcommand(flow_document.New())
	cmd.AddSubcommand(github_actions.New())
	cmd.AddSubcommand(json.New())
	cmd.AddSubcommand(run.New())
	cmd.AddSubcommand(shell.New())
	cmd.AddSubcommand(yaml.New())
	cmd.AddSubcommand(golang.New())
	return cmd
}
