package json

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/wakuwaku3/flame/cli/internal/check/schemavalidate"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

func New() clix.Subcommand {
	return clix.NewLeaf("json", "JSON ファイルを検査する", Run)
}

func Run(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check json <json_file>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	failed := false
	for _, file := range args {
		if reason := checkJSON(file); reason != "" {
			fmt.Fprintf(in.Stderr(), "FAIL: %s: %s\n", file, reason)
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// checkJSON は path の JSON を検査し、 失敗時に stderr に出す reason を返す
// (成功時は空文字)。 syntax 検査 (= 既存挙動) に加え、 vendor/flame/schemas/
// に対応 schema が存在する場合は flame 独自型と判定して `$schema` property の
// 必須化と schema validation を行う (FLM_APP_0003 §flame 独自型の schema 規約)。
func checkJSON(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "invalid JSON"
	}
	var value any
	if uerr := json.Unmarshal(data, &value); uerr != nil {
		return "invalid JSON"
	}
	schemaPath, hasSchema, err := schemavalidate.FindFlameSchema(path, "json")
	if err != nil {
		return fmt.Sprintf("schema lookup failed: %s", err)
	}
	if !hasSchema {
		return ""
	}
	directive := extractJSONSchemaProp(value)
	if directive == "" {
		return fmt.Sprintf(`missing top-level "$schema" property (flame schema available at %s)`, schemaPath)
	}
	resolved, err := schemavalidate.ResolveDirectivePath(path, directive)
	if err != nil {
		return fmt.Sprintf("invalid schema property path: %s", err)
	}
	if resolved != schemaPath {
		return fmt.Sprintf("$schema points to %s but flame schema is at %s", resolved, schemaPath)
	}
	// `$schema` property は IDE / lint 用 metadata であり対象 type の data ではないため、
	// validation 入力からは取り除く (= 各 schema 著者が `$schema` を allowed property として
	// 明示する負担を避ける)。
	stripped := stripJSONSchemaProp(value)
	sch, err := schemavalidate.CompileSchema(schemaPath)
	if err != nil {
		return fmt.Sprintf("schema compile failed: %s", err)
	}
	if verr := sch.Validate(schemavalidate.Normalize(stripped)); verr != nil {
		return fmt.Sprintf("schema validation failed: %s", verr)
	}
	return ""
}

func extractJSONSchemaProp(value any) string {
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	v, ok := m["$schema"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func stripJSONSchemaProp(value any) any {
	m, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == "$schema" {
			continue
		}
		out[k] = v
	}
	return out
}
