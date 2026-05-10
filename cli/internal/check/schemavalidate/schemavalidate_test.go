package schemavalidate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/check/schemavalidate"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	t.Run("scalar 値はそのまま返す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		input := "hello"

		// Act
		got := schemavalidate.Normalize(input)

		// Assert
		assert.Equal(t, "hello", got)
	})

	t.Run("map[string]any は再帰的に Normalize される (= 値側の map[any]any も string 化される)", func(t *testing.T) {
		t.Parallel()

		// Arrange
		input := map[string]any{
			"k1": "v1",
			"k2": map[any]any{1: "one"},
		}

		// Act
		got := schemavalidate.Normalize(input)

		// Assert
		expected := map[string]any{
			"k1": "v1",
			"k2": map[string]any{"1": "one"},
		}
		assert.Equal(t, expected, got)
	})

	t.Run("map[any]any は string キーに変換される", func(t *testing.T) {
		t.Parallel()

		// Arrange
		input := map[any]any{1: "one", "two": 2}

		// Act
		got := schemavalidate.Normalize(input)

		// Assert
		expected := map[string]any{"1": "one", "two": 2}
		assert.Equal(t, expected, got)
	})

	t.Run("[]any は要素を再帰的に Normalize する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		input := []any{
			"a",
			map[any]any{1: "one"},
			[]any{map[any]any{"nested": "deep"}},
		}

		// Act
		got := schemavalidate.Normalize(input)

		// Assert
		expected := []any{
			"a",
			map[string]any{"1": "one"},
			[]any{map[string]any{"nested": "deep"}},
		}
		assert.Equal(t, expected, got)
	})
}

func TestFindFlameSchema(t *testing.T) {
	t.Parallel()

	t.Run("schema 不在なら (\"\", false, nil)", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		target := filepath.Join(dir, "foo.yaml")

		// Act
		path, found, err := schemavalidate.FindFlameSchema(target, "yaml")

		// Assert
		require.NoError(t, err)
		assert.False(t, found)
		assert.Empty(t, path)
	})

	t.Run("同階層に schema があれば (path, true, nil)", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemasDir := filepath.Join(dir, "vendor", "flame", "schemas")
		require.NoError(t, os.MkdirAll(schemasDir, 0o750))
		schemaPath := filepath.Join(schemasDir, "foo.yaml.schema.yaml")
		require.NoError(t, os.WriteFile(schemaPath, []byte(""), 0o600))
		target := filepath.Join(dir, "foo.yaml")

		// Act
		path, found, err := schemavalidate.FindFlameSchema(target, "yaml")

		// Assert
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, schemaPath, path)
	})

	t.Run("親階層に schema があれば search-upward で発見する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		schemasDir := filepath.Join(dir, "vendor", "flame", "schemas")
		require.NoError(t, os.MkdirAll(schemasDir, 0o750))
		schemaPath := filepath.Join(schemasDir, "foo.yaml.schema.yaml")
		require.NoError(t, os.WriteFile(schemaPath, []byte(""), 0o600))
		nested := filepath.Join(dir, "a", "b", "c")
		require.NoError(t, os.MkdirAll(nested, 0o750))
		target := filepath.Join(nested, "foo.yaml")

		// Act
		path, found, err := schemavalidate.FindFlameSchema(target, "yaml")

		// Assert
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, schemaPath, path)
	})
}

func TestResolveDirectivePath(t *testing.T) {
	t.Parallel()

	t.Run("absolute path は Clean のみで返す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		filePath := "/tmp/foo.yaml"
		directive := "/abs/path/to/schema.yaml"

		// Act
		got, err := schemavalidate.ResolveDirectivePath(filePath, directive)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "/abs/path/to/schema.yaml", got)
	})

	t.Run("relative path は filePath の親 dir に対して解決する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		filePath := filepath.Join(dir, "foo.yaml")
		directive := "./vendor/flame/schemas/foo.yaml.schema.yaml"

		// Act
		got, err := schemavalidate.ResolveDirectivePath(filePath, directive)

		// Assert
		require.NoError(t, err)
		expected := filepath.Join(dir, "vendor", "flame", "schemas", "foo.yaml.schema.yaml")
		assert.Equal(t, expected, got)
	})

	t.Run("親方向 path (..) も Clean されて解決される", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		filePath := filepath.Join(dir, "sub", "foo.yaml")
		directive := "../vendor/flame/schemas/foo.yaml.schema.yaml"

		// Act
		got, err := schemavalidate.ResolveDirectivePath(filePath, directive)

		// Assert
		require.NoError(t, err)
		expected := filepath.Join(dir, "vendor", "flame", "schemas", "foo.yaml.schema.yaml")
		assert.Equal(t, expected, got)
	})
}
