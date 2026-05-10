package summary

import (
	"context"
	"fmt"
	"os"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const exitCodeFailure = 1

func New() clix.Subcommand {
	return clix.NewLeaf("summary", "release matrix の結果を集約する", run)
}

func run(_ context.Context, in clix.RunInput) error {
	enumerate, err := requireEnv("ENUMERATE_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}
	release, err := requireEnv("RELEASE_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}
	noop, err := requireEnv("NOOP_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}
	if enumerate == "success" && isOkTerminal(release) && isOkTerminal(noop) {
		fmt.Fprintf(in.Stdout(), "deploy: ok (enumerate=%s release=%s noop=%s)\n", enumerate, release, noop)
		return nil
	}
	fmt.Fprintf(in.Stderr(), "deploy: failure (enumerate=%s release=%s noop=%s)\n", enumerate, release, noop)
	return ex.Wrap(clix.NewExitError(exitCodeFailure))
}

func isOkTerminal(v string) bool {
	return v == "success" || v == "skipped"
}

func requireEnv(name string) (string, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", ex.Errorf("%s must be set", name)
	}
	return v, nil
}
