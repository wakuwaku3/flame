package json_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/json"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("全て妥当な JSON ファイルなら no-op success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		validA := filepath.Join(dir, "a.json")
		validB := filepath.Join(dir, "b.json")
		require.NoError(t, os.WriteFile(validA, []byte(`{"a":1}`), 0o600))
		require.NoError(t, os.WriteFile(validB, []byte(`[1,2,3]`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", validA, validB})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("不正な JSON ファイルがあると FAIL 行を stderr に出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		valid := filepath.Join(dir, "valid.json")
		invalid := filepath.Join(dir, "invalid.json")
		require.NoError(t, os.WriteFile(valid, []byte(`{}`), 0o600))
		require.NoError(t, os.WriteFile(invalid, []byte(`{`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", valid, invalid})
		expectedStderr := "FAIL: " + invalid + ": invalid JSON\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("複数の不正 JSON は全件 FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		invalidA := filepath.Join(dir, "a.json")
		invalidB := filepath.Join(dir, "b.json")
		require.NoError(t, os.WriteFile(invalidA, []byte(`{`), 0o600))
		require.NoError(t, os.WriteFile(invalidB, []byte(`}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", invalidA, invalidB})
		expectedStderr := "FAIL: " + invalidA + ": invalid JSON\nFAIL: " + invalidB + ": invalid JSON\n"

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
		missing := filepath.Join(t.TempDir(), "missing.json")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", missing})
		expectedStderr := "FAIL: " + missing + ": invalid JSON\n"

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
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json"})
		const expectedStderr = "usage: flame check json <json_file>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("flame schema 併設 + $schema property 正常 + conforms なら success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		writeFlameJSONSchema(t, dir, `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["hello"],
  "additionalProperties": false,
  "properties": {
    "hello": {"type": "string"}
  }
}`)
		target := filepath.Join(dir, "foo.json")
		require.NoError(t, os.WriteFile(target, []byte(`{"$schema":"./vendor/flame/schemas/foo.json.schema.json","hello":"world"}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", target})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("flame schema 併設 + $schema property 不在なら FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemaPath := writeFlameJSONSchema(t, dir, `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object"
}`)
		target := filepath.Join(dir, "foo.json")
		require.NoError(t, os.WriteFile(target, []byte(`{"hello":"world"}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", target})
		expectedStderr := "FAIL: " + target + `: missing top-level "$schema" property (flame schema available at ` + schemaPath + ")\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("flame schema 併設 + $schema が別 path を指すと FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemaPath := writeFlameJSONSchema(t, dir, `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object"
}`)
		target := filepath.Join(dir, "foo.json")
		require.NoError(t, os.WriteFile(target, []byte(`{"$schema":"./other.schema.json","hello":"world"}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", target})
		expectedDirective := filepath.Join(dir, "other.schema.json")
		expectedStderr := "FAIL: " + target + ": $schema points to " + expectedDirective + " but flame schema is at " + schemaPath + "\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("flame schema 併設 + $schema 正常 + conforms せずなら FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		writeFlameJSONSchema(t, dir, `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["hello"],
  "additionalProperties": false,
  "properties": {
    "hello": {"type": "string"}
  }
}`)
		target := filepath.Join(dir, "foo.json")
		require.NoError(t, os.WriteFile(target, []byte(`{"$schema":"./vendor/flame/schemas/foo.json.schema.json","hello":123}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", target})
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

	t.Run("flame schema 併設 + top-level が array なら $schema property 不在扱いで FAIL", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemaPath := writeFlameJSONSchema(t, dir, `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "array"
}`)
		target := filepath.Join(dir, "foo.json")
		// top-level が JSON object でない (= array / scalar) 場合、 `$schema` property は
		// 構造上配置できず production は「不在扱い」 として FAIL する。 当該経路を service-level
		// で焼き付ける。
		require.NoError(t, os.WriteFile(target, []byte(`[1,2,3]`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(json.New())
		fake := clix.NewFakeIO(t, []string{"json", target})
		expectedStderr := "FAIL: " + target + `: missing top-level "$schema" property (flame schema available at ` + schemaPath + ")\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})
}

func writeFlameJSONSchema(tb testing.TB, dir, content string) string {
	tb.Helper()
	schemasDir := filepath.Join(dir, "vendor", "flame", "schemas")
	require.NoError(tb, os.MkdirAll(schemasDir, 0o750))
	path := filepath.Join(schemasDir, "foo.json.schema.json")
	require.NoError(tb, os.WriteFile(path, []byte(content), 0o600))
	return path
}
