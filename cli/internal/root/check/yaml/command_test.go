package yaml_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/yaml"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("全て妥当な YAML ファイルなら no-op success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		validA := filepath.Join(dir, "a.yaml")
		validB := filepath.Join(dir, "b.yaml")
		require.NoError(t, os.WriteFile(validA, []byte("a: 1\nb: 2\n"), 0o600))
		require.NoError(t, os.WriteFile(validB, []byte("- 1\n- 2\n- 3\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", validA, validB})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("不正な YAML ファイルがあると FAIL 行を stderr に出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		valid := filepath.Join(dir, "valid.yaml")
		invalid := filepath.Join(dir, "invalid.yaml")
		require.NoError(t, os.WriteFile(valid, []byte("a: 1\n"), 0o600))
		require.NoError(t, os.WriteFile(invalid, []byte("a: [1, 2\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", valid, invalid})
		expectedStderr := "FAIL: " + invalid + ": invalid YAML\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("複数の不正 YAML は全件 FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		invalidA := filepath.Join(dir, "a.yaml")
		invalidB := filepath.Join(dir, "b.yaml")
		require.NoError(t, os.WriteFile(invalidA, []byte("a: [1, 2\n"), 0o600))
		require.NoError(t, os.WriteFile(invalidB, []byte("\ta: 1\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", invalidA, invalidB})
		expectedStderr := "FAIL: " + invalidA + ": invalid YAML\nFAIL: " + invalidB + ": invalid YAML\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("読み込めないパスも invalid 扱いで FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		missing := filepath.Join(t.TempDir(), "missing.yaml")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", missing})
		expectedStderr := "FAIL: " + missing + ": invalid YAML\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("引数なし起動は usage を stderr に出して exit code 2 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml"})
		const expectedStderr = "usage: flame check yaml <yaml_file>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("flame schema 併設 + directive 正常 + conforms なら success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		writeFlameYAMLSchema(t, dir, `---
$schema: https://json-schema.org/draft/2020-12/schema
type: object
required:
  - hello
additionalProperties: false
properties:
  hello:
    type: string
`)
		target := filepath.Join(dir, "foo.yaml")
		require.NoError(t, os.WriteFile(target, []byte(`---
# yaml-language-server: $schema=./vendor/flame/schemas/foo.yaml.schema.yaml
hello: world
`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", target})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("flame schema 併設 + directive 不在なら FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemaPath := writeFlameYAMLSchema(t, dir, `---
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  hello:
    type: string
`)
		target := filepath.Join(dir, "foo.yaml")
		require.NoError(t, os.WriteFile(target, []byte("hello: world\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", target})
		expectedStderr := "FAIL: " + target + ": missing yaml-language-server $schema directive (flame schema available at " + schemaPath + ")\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("flame schema 併設 + directive が別 path を指すと FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemaPath := writeFlameYAMLSchema(t, dir, `---
$schema: https://json-schema.org/draft/2020-12/schema
type: object
`)
		target := filepath.Join(dir, "foo.yaml")
		require.NoError(t, os.WriteFile(target, []byte(`---
# yaml-language-server: $schema=./other.schema.yaml
hello: world
`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", target})
		expectedDirective := filepath.Join(dir, "other.schema.yaml")
		expectedStderr := "FAIL: " + target + ": schema directive points to " + expectedDirective + " but flame schema is at " + schemaPath + "\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("flame schema 併設 + directive 正常 + conforms せずなら FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		writeFlameYAMLSchema(t, dir, `---
$schema: https://json-schema.org/draft/2020-12/schema
type: object
required:
  - hello
additionalProperties: false
properties:
  hello:
    type: string
`)
		target := filepath.Join(dir, "foo.yaml")
		require.NoError(t, os.WriteFile(target, []byte(`---
# yaml-language-server: $schema=./vendor/flame/schemas/foo.yaml.schema.yaml
hello: 123
`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", target})
		// stderr 全体は jsonschema/v6 の error 文面に依存し library version で揺れるため、
		// production の出力契約として確定している部分 (FAIL prefix + 違反位置 `/hello` の言及)
		// を 1 行 (`\n$`) として正規表現で全体焼き付けする (FLM_APP_0009 §assertion 規約)。
		expectedRe := regexp.MustCompile(`(?s)^FAIL: ` + regexp.QuoteMeta(target) + `: schema validation failed:.*'/hello'.*\n$`)

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		// stderr は library version で揺れる文面のため per-channel 検査だが、 stdout 側の漏れを
		// 検出するため空であることを別途 assert する (FLM_APP_0009 §assertion 規約 の合算検証
		// default を per-channel 経路でも等価に保つ)。
		assert.Regexp(t, expectedRe, fake.StderrString(t))
		assert.Empty(t, fake.StdoutString(t))
	})

	t.Run("flame schema 併設 + directive が scan 範囲外 (6 行目以降) なら不在扱いで FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemaPath := writeFlameYAMLSchema(t, dir, `---
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  hello:
    type: string
`)
		target := filepath.Join(dir, "foo.yaml")
		// directive を意図的に 6 行目に置き、 5 行 scan の境界外として「不在扱い」 になることを焼き付ける。
		require.NoError(t, os.WriteFile(target, []byte(`---
# leading comment line 2
# leading comment line 3
# leading comment line 4
# leading comment line 5
# yaml-language-server: $schema=./vendor/flame/schemas/foo.yaml.schema.yaml
hello: world
`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(yaml.New())
		fake := clix.NewFakeIO(t, []string{"yaml", target})
		expectedStderr := "FAIL: " + target + ": missing yaml-language-server $schema directive (flame schema available at " + schemaPath + ")\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})
}

func writeFlameYAMLSchema(tb testing.TB, dir, content string) string {
	tb.Helper()
	schemasDir := filepath.Join(dir, "vendor", "flame", "schemas")
	require.NoError(tb, os.MkdirAll(schemasDir, 0o750))
	path := filepath.Join(schemasDir, "foo.yaml.schema.yaml")
	require.NoError(tb, os.WriteFile(path, []byte(content), 0o600))
	return path
}
