package devbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/devbox"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("引数なし起動は usage を stderr に出して exit code 2 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox"})
		const expectedStderr = "usage: flame check devbox <devbox.json|devbox.lock>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("array packages で declared と locked が一致すれば success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go@1.22.0","jq@1.7"]}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{
  "packages": {
    "go@1.22.0": {"resolved": "..."},
    "jq@1.7": {"resolved": "..."},
    "github:NixOS/nixpkgs/abcdef": {"resolved": "..."}
  }
}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("object packages も name@version 列に正規化されて一致すれば success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":{"go":"1.22.0","jq":"1.7"}}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{
  "packages": {
    "go@1.22.0": {},
    "jq@1.7": {}
  }
}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("@latest など non-concrete version は FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go@latest"]}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{"packages":{"go@latest":{}}}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})
		expectedStderr := "FAIL: " + jsonPath + ": package 'go@latest' uses non-concrete version 'latest' (FLM_ENG_0002 forbids floating specs like @latest)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("@<version> 不在の package は FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go"]}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{"packages":{}}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})
		// shell 版は @<version> 不在の package を declared list に残したまま lock 比較に進めるため、 version 違反 と missing-from-lock の 2 行が出る挙動 (互換のため Go 版もこれを踏襲)。
		expectedStderr := "FAIL: " + jsonPath + ": package 'go' has no '@<version>' (FLM_ENG_0002 requires an explicit version)\n" +
			"FAIL: " + jsonPath + ": package 'go' is declared but missing from " + lockPath + " (run 'devbox install' to update the lock)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("devbox.lock が無い場合は FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go@1.22.0"]}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})
		lockPath := filepath.Join(dir, "devbox.lock")
		expectedStderr := "FAIL: " + jsonPath + ": corresponding devbox.lock not found at " + lockPath + " (FLM_ENG_0002 requires the lock to be tracked)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("declared に在り locked に無い package は missing FAIL を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go@1.22.0","jq@1.7"]}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{"packages":{"go@1.22.0":{}}}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})
		expectedStderr := "FAIL: " + jsonPath + ": package 'jq@1.7' is declared but missing from " + lockPath + " (run 'devbox install' to update the lock)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("locked にあるが declared に無い package は stale FAIL を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go@1.22.0"]}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{"packages":{"go@1.22.0":{},"jq@1.7":{}}}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})
		expectedStderr := "FAIL: " + lockPath + ": stale entry 'jq@1.7' is locked but no longer declared in " + jsonPath + " (run 'devbox install' to update the lock)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("flake ref のみ含む lock key は declared 比較対象外で missing FAIL とならない", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go@1.22.0"]}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{
  "packages": {
    "go@1.22.0": {},
    "github:NixOS/nixpkgs/abcdef": {}
  }
}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("同一 dir の重複引数は 1 回しか検査されない", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		lockPath := filepath.Join(dir, "devbox.lock")
		require.NoError(t, os.WriteFile(jsonPath, []byte(`{"packages":["go@latest"]}`), 0o600))
		require.NoError(t, os.WriteFile(lockPath, []byte(`{"packages":{"go@latest":{}}}`), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath, lockPath})
		expectedStderr := "FAIL: " + jsonPath + ": package 'go@latest' uses non-concrete version 'latest' (FLM_ENG_0002 forbids floating specs like @latest)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("devbox.json が読めなければ FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "devbox.json")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(devbox.New())
		fake := clix.NewFakeIO(t, []string{"devbox", jsonPath})
		expectedStderr := "FAIL: " + jsonPath + ": not readable\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})
}
