package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeVendorTreeHash(t *testing.T) {
	t.Parallel()

	t.Run("同一内容の tree は同一 hash を返す (decoupled from walk order)", func(t *testing.T) {
		t.Parallel()
		root1 := buildHashFixture(t, map[string]string{
			"a.txt":         "alpha\n",
			"b/c.txt":       "charlie\n",
			"sub/dir/x.txt": "ex\n",
		})
		root2 := buildHashFixture(t, map[string]string{
			"sub/dir/x.txt": "ex\n",
			"a.txt":         "alpha\n",
			"b/c.txt":       "charlie\n",
		})

		h1, err := ComputeVendorTreeHash(context.Background(), root1)
		require.NoError(t, err)
		h2, err := ComputeVendorTreeHash(context.Background(), root2)
		require.NoError(t, err)
		assert.Equal(t, h1, h2)
		assert.True(t, strings.HasPrefix(h1, "sha256:"))
	})

	t.Run("file 内容が変わると hash が変わる", func(t *testing.T) {
		t.Parallel()
		root := buildHashFixture(t, map[string]string{"a.txt": "v1\n"})
		h1, err := ComputeVendorTreeHash(context.Background(), root)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("v2\n"), 0o644))
		h2, err := ComputeVendorTreeHash(context.Background(), root)
		require.NoError(t, err)

		assert.NotEqual(t, h1, h2)
	})

	t.Run("file が追加されると hash が変わる", func(t *testing.T) {
		t.Parallel()
		root := buildHashFixture(t, map[string]string{"a.txt": "v1\n"})
		h1, err := ComputeVendorTreeHash(context.Background(), root)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("v1\n"), 0o644))
		h2, err := ComputeVendorTreeHash(context.Background(), root)
		require.NoError(t, err)

		assert.NotEqual(t, h1, h2)
	})

	t.Run("空 dir でも sha256: prefix の hash を返す", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		h, err := ComputeVendorTreeHash(context.Background(), root)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(h, "sha256:"))
	})
}

func buildHashFixture(tb testing.TB, files map[string]string) string {
	tb.Helper()
	root := tb.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		require.NoError(tb, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(tb, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}
