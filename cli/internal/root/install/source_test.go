package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeVendorReadOnly(t *testing.T) {
	t.Parallel()

	t.Run("file は 444 / dir は 555 で readonly 化する", func(t *testing.T) {
		t.Parallel()
		root := buildVendorTree(t)
		// t.TempDir() cleanup が unlink できるよう、 test 終了前に writable に戻す
		t.Cleanup(func() {
			_ = makeVendorWritable(context.Background(), root)
		})

		require.NoError(t, makeVendorReadOnly(context.Background(), root))

		assertPerm(t, root, 0o555)
		assertPerm(t, filepath.Join(root, ".claude"), 0o555)
		assertPerm(t, filepath.Join(root, "CLAUDE.md"), 0o444)
		assertPerm(t, filepath.Join(root, ".claude", "rules", "x.md"), 0o444)
	})
}

func TestMakeVendorWritable(t *testing.T) {
	t.Parallel()

	t.Run("readonly 化された tree を 644/755 に戻せる", func(t *testing.T) {
		t.Parallel()
		root := buildVendorTree(t)
		require.NoError(t, makeVendorReadOnly(context.Background(), root))

		require.NoError(t, makeVendorWritable(context.Background(), root))

		assertPerm(t, root, 0o755)
		assertPerm(t, filepath.Join(root, ".claude"), 0o755)
		assertPerm(t, filepath.Join(root, "CLAUDE.md"), 0o644)
		// 書き込みも復活
		require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("after"), 0o644))
	})
}

func buildVendorTree(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	require.NoError(tb, os.MkdirAll(filepath.Join(root, ".claude", "rules"), 0o755))
	require.NoError(tb, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("vendor claude\n"), 0o644))
	require.NoError(tb, os.WriteFile(filepath.Join(root, ".claude", "rules", "x.md"), []byte("rule\n"), 0o644))
	return root
}

func assertPerm(tb testing.TB, path string, expected os.FileMode) {
	tb.Helper()
	info, err := os.Stat(path)
	require.NoError(tb, err)
	assert.Equal(tb, expected, info.Mode().Perm(), "perm mismatch for %s", path)
}
