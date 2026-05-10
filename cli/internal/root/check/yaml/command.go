package yaml

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/cli/internal/check/schemavalidate"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage      = 2
	exitCodeFailure    = 1
	directiveScanLines = 5
)

// directiveRe は yaml-language-server 仕様 (`# yaml-language-server: $schema=<path>`)
// と整合する形で directive を抽出する。 空白の揺れ (= 周辺の任意 whitespace) を
// 許容することで lint の運用上の摩擦を抑える。
var directiveRe = regexp.MustCompile(`^#\s*yaml-language-server\s*:\s*\$schema\s*=\s*(\S+)\s*$`)

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
		if reason := checkYAML(file); reason != "" {
			fmt.Fprintf(in.Stderr(), "FAIL: %s: %s\n", file, reason)
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// checkYAML は path の YAML を検査し、 失敗時に stderr に出す reason を返す
// (成功時は空文字)。 syntax 検査 (= 既存挙動) に加え、 vendor/flame/schemas/
// に対応 schema が存在する場合は flame 独自型と判定して directive 必須化と
// schema validation を行う (FLM_APP_0004 §flame 独自型の schema 規約)。
func checkYAML(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "invalid YAML"
	}
	var value any
	if uerr := yaml.Unmarshal(data, &value); uerr != nil {
		return "invalid YAML"
	}
	schemaPath, hasSchema, err := schemavalidate.FindFlameSchema(path, "yaml")
	if err != nil {
		return fmt.Sprintf("schema lookup failed: %s", err)
	}
	if !hasSchema {
		return ""
	}
	directive := extractYAMLDirective(data)
	if directive == "" {
		return fmt.Sprintf("missing yaml-language-server $schema directive (flame schema available at %s)", schemaPath)
	}
	resolved, err := schemavalidate.ResolveDirectivePath(path, directive)
	if err != nil {
		return fmt.Sprintf("invalid schema directive path: %s", err)
	}
	if resolved != schemaPath {
		return fmt.Sprintf("schema directive points to %s but flame schema is at %s", resolved, schemaPath)
	}
	sch, err := schemavalidate.CompileSchema(schemaPath)
	if err != nil {
		return fmt.Sprintf("schema compile failed: %s", err)
	}
	if verr := sch.Validate(schemavalidate.Normalize(value)); verr != nil {
		return fmt.Sprintf("schema validation failed: %s", verr)
	}
	return ""
}

func extractYAMLDirective(data []byte) string {
	lines := bytes.SplitN(data, []byte("\n"), directiveScanLines+1)
	limit := min(len(lines), directiveScanLines)
	for i := range limit {
		line := strings.TrimRight(string(lines[i]), "\r")
		if m := directiveRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}
