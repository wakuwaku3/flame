package yaml_test

import (
	"os"
	"path/filepath"
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
}
