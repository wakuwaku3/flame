package json_test

import (
	"os"
	"path/filepath"
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
}
