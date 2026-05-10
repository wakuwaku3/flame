package build_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/build"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("複数 main package が build 成功すれば no-op success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		moduleRoot := newGoModule(t)
		dirA := filepath.Join(moduleRoot, "cmd", "a")
		dirB := filepath.Join(moduleRoot, "cmd", "b")
		require.NoError(t, os.MkdirAll(dirA, 0o750))
		require.NoError(t, os.MkdirAll(dirB, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dirA, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dirB, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(build.New())
		fake := clix.NewFakeIO(t, []string{"build", dirA, dirB})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("引数が dir でなければ FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		missing := filepath.Join(t.TempDir(), "missing")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(build.New())
		fake := clix.NewFakeIO(t, []string{"build", missing})
		expectedStderr := "FAIL: " + missing + ": not a directory\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("親方向に go.mod が無い dir は FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		// t.TempDir() 配下には go.mod が無いため、 ここから親方向に辿っても module root が見つからず、 (root まで遡っても go.mod 不在) で FAIL する経路を踏ませる。
		isolated := t.TempDir()
		pkgDir := filepath.Join(isolated, "pkg")
		require.NoError(t, os.Mkdir(pkgDir, 0o750))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(build.New())
		fake := clix.NewFakeIO(t, []string{"build", pkgDir})
		expectedStderr := "FAIL: " + pkgDir + ": 親方向に go.mod が見つからない\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("compile error を持つ package は FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		moduleRoot := newGoModule(t)
		pkgDir := filepath.Join(moduleRoot, "cmd", "broken")
		require.NoError(t, os.MkdirAll(pkgDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte("package main\n\nfunc main() {\n\tundefinedSymbol()\n}\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(build.New())
		fake := clix.NewFakeIO(t, []string{"build", pkgDir})
		expectedStderr := "FAIL: " + pkgDir + ": go build ./cmd/broken が失敗した\n"

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
		r.AddCommand(build.New())
		fake := clix.NewFakeIO(t, []string{"build"})
		const expectedStderr = "usage: flame check go build <main_package_dir>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})
}

// newGoModule は t.TempDir() 配下に最小 go.mod を持つ module を作って root 絶対パスを返す。 service-level test 内で main package を実際に build する経路を駆動する目的のため、 production code の helper ではなく test 専用 helper として配置する。
func newGoModule(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	const goMod = "module flame_check_go_build_test\n\ngo 1.26\n"
	require.NoError(tb, os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o600))
	return root
}
