package summary

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const exitCodeFailure = 1

func New() clix.Subcommand {
	return clix.NewLeaf("summary", "checker matrix の結果を集約する", run)
}

func run(_ context.Context, in clix.RunInput) error {
	bucket, err := requireEnv("BUCKET_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}
	check, err := requireEnv("CHECK_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}
	noop, err := requireEnv("NOOP_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}
	label, err := requireEnv("LABEL_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}
	installDrift, err := requireEnv("INSTALL_DRIFT_RESULT")
	if err != nil {
		return ex.Wrap(err)
	}

	if bucket != "success" {
		fmt.Fprintf(in.Stderr(), "bucket job did not succeed (result=%s)\n", bucket)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	for _, pair := range []struct{ name, value string }{
		{"CHECK", check},
		{"NOOP", noop},
		{"LABEL", label},
		{"INSTALL_DRIFT", installDrift},
	} {
		if !isOkTerminal(pair.value) {
			return failJob(in.Stderr(), pair.name, pair.value)
		}
	}
	if check != "success" && noop != "success" {
		fmt.Fprintln(in.Stderr(), "neither check nor noop succeeded")
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	fmt.Fprintf(in.Stdout(), "OK: bucket=%s check=%s noop=%s label=%s install_drift=%s\n", bucket, check, noop, label, installDrift)
	return nil
}

func failJob(stderr io.Writer, name, value string) error {
	fmt.Fprintf(stderr, "%s job result was '%s' (expected success or skipped)\n", name, value)
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
