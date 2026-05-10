package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/test"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("test 0 件の package でも go test が success すれば exit 0", func(t *testing.T) {
		t.Parallel()

		// Arrange
		moduleRoot := t.TempDir()
		writeGoModule(t, moduleRoot, "example.com/notest")
		writeFile(t, filepath.Join(moduleRoot, "lib.go"), "package notest\n\nfunc Add(a, b int) int { return a + b }\n")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(test.New())
		fake := clix.NewFakeIO(t, []string{"test", moduleRoot})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
	})

	t.Run("test が pass する package は exit 0", func(t *testing.T) {
		t.Parallel()

		// Arrange
		moduleRoot := t.TempDir()
		writeGoModule(t, moduleRoot, "example.com/passing")
		writeFile(t, filepath.Join(moduleRoot, "lib.go"), "package passing\n\nfunc Add(a, b int) int { return a + b }\n")
		writeFile(t, filepath.Join(moduleRoot, "lib_test.go"), `package passing

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("expected 3")
	}
}
`)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(test.New())
		fake := clix.NewFakeIO(t, []string{"test", moduleRoot})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
	})

	t.Run("test が fail する package は FAIL 行 + exit 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		moduleRoot := t.TempDir()
		writeGoModule(t, moduleRoot, "example.com/failing")
		writeFile(t, filepath.Join(moduleRoot, "lib.go"), "package failing\n\nfunc Add(a, b int) int { return a + b }\n")
		writeFile(t, filepath.Join(moduleRoot, "lib_test.go"), `package failing

import "testing"

func TestAdd(t *testing.T) {
	t.Fatal("forced failure")
}
`)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(test.New())
		fake := clix.NewFakeIO(t, []string{"test", moduleRoot})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
	})

	t.Run("存在しない dir は FAIL 行 + exit 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		missing := filepath.Join(t.TempDir(), "missing")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(test.New())
		fake := clix.NewFakeIO(t, []string{"test", missing})
		expectedStderr := "FAIL: " + missing + ": not a directory\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("go.mod が親方向に存在しない dir は FAIL 行 + exit 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		moduleLessRoot := t.TempDir()
		moduleLessDir := filepath.Join(moduleLessRoot, "pkg")
		require.NoError(t, os.MkdirAll(moduleLessDir, 0o750))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(test.New())
		fake := clix.NewFakeIO(t, []string{"test", moduleLessDir})
		expectedStderr := "FAIL: " + moduleLessDir + ": 親方向に go.mod が見つからない\n"

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
		r.AddCommand(test.New())
		fake := clix.NewFakeIO(t, []string{"test"})
		const expectedStderr = "usage: flame check go test <test_package_dir>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})
}

func writeGoModule(tb testing.TB, dir, modulePath string) {
	tb.Helper()
	content := "module " + modulePath + "\n\ngo 1.22\n"
	writeFile(tb, filepath.Join(dir, "go.mod"), content)
}

func writeFile(tb testing.TB, path, content string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(path, []byte(content), 0o600))
}
