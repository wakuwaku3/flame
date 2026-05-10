package lint_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/lint"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Run("引数なし起動は usage を stderr に出して exit code 2 を伝搬する", func(t *testing.T) {
		// FLAME_CHECKER_MODE を弄る subtest と直列化する必要があるため t.Parallel しない (env は process global)。
		t.Setenv("FLAME_CHECKER_MODE", "")

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(lint.New())
		fake := clix.NewFakeIO(t, []string{"lint"})
		const expectedStderr = "usage: flame check go lint <package_dir>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("FLAME_CHECKER_MODE が fix / diagnose 以外なら error 行を出して exit code 2 を伝搬する", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "invalid")

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(lint.New())
		fake := clix.NewFakeIO(t, []string{"lint", "."})
		const expectedStderr = "error: invalid FLAME_CHECKER_MODE='invalid' (expected 'fix' or 'diagnose')\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("存在しない dir は FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")

		// Arrange
		missing := filepath.Join(t.TempDir(), "missing")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(lint.New())
		fake := clix.NewFakeIO(t, []string{"lint", missing})
		expectedStderr := "FAIL: " + missing + ": not a directory\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("go.mod が親方向に無い dir は FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")

		// Arrange
		// t.TempDir() は OS temp dir 配下 (例 /tmp) に作られ、 親方向には go.mod が存在しないため、 「親方向 go.mod 不在」 経路の検証に使える。
		target := filepath.Join(t.TempDir(), "pkg")
		require.NoError(t, os.Mkdir(target, 0o700))

		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(lint.New())
		fake := clix.NewFakeIO(t, []string{"lint", target})
		expectedStderr := "FAIL: " + target + ": 親方向に go.mod が見つからない\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("複数の不正な dir はすべて FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")

		// Arrange
		base := t.TempDir()
		missing := filepath.Join(base, "missing")
		noGoMod := filepath.Join(base, "pkg")
		require.NoError(t, os.Mkdir(noGoMod, 0o700))

		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(lint.New())
		fake := clix.NewFakeIO(t, []string{"lint", missing, noGoMod})
		expectedStderr := "FAIL: " + missing + ": not a directory\n" +
			"FAIL: " + noGoMod + ": 親方向に go.mod が見つからない\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})
}
