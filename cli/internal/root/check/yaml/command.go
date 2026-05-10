package yaml

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

func New() clix.Subcommand {
	return clix.NewLeaf("yaml", "YAML ファイルを検査する", Run)
}

func Run(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check yaml <yaml_file>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	failed := false
	for _, file := range args {
		if !isValidYAMLFile(file) {
			fmt.Fprintf(in.Stderr(), "FAIL: %s: invalid YAML\n", file)
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

func isValidYAMLFile(path string) bool {
	data, err := os.ReadFile(path) //nolint:gosec // G304: flame check yaml が受け取る path は CLI 起動時 argv そのもので、 検査対象として外部入力を読み込むのが endpoint の責務 (= shell の `yamllint --strict -- "$file"` と同等の挙動を Go で再現するため、 path は意図的に caller 制御下の任意値)。
	if err != nil {
		return false
	}
	var node yaml.Node
	return yaml.Unmarshal(data, &node) == nil
}
