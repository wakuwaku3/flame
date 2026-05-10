package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
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

		assertPerm(t, root, readOnlyDirPerm)
		assertPerm(t, filepath.Join(root, ".claude"), readOnlyDirPerm)
		assertPerm(t, filepath.Join(root, "CLAUDE.md"), readOnlyPerm)
		assertPerm(t, filepath.Join(root, ".claude", "rules", "x.md"), readOnlyPerm)
	})
}

func TestNeedsRefetch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		prev *Lock
		m    *Manifest
		name string
		want bool
	}{
		{
			name: "前回 install 記録なしは false (旧 schema / 初回)",
			prev: &Lock{Installed: nil, Files: nil, Embeds: nil},
			m:    &Manifest{Source: "github.com/wakuwaku3/flame", Version: "v1.0.0", Ignore: nil, Stage1ExtraAgents: nil},
			want: false,
		},
		{
			name: "前回と同一 source / version は false",
			prev: &Lock{Installed: &LockInstalled{Source: "github.com/wakuwaku3/flame", Version: "v1.0.0", TreeHash: "sha256:x"}, Files: nil, Embeds: nil},
			m:    &Manifest{Source: "github.com/wakuwaku3/flame", Version: "v1.0.0", Ignore: nil, Stage1ExtraAgents: nil},
			want: false,
		},
		{
			name: "version が異なれば true",
			prev: &Lock{Installed: &LockInstalled{Source: "github.com/wakuwaku3/flame", Version: "v1.0.0", TreeHash: "sha256:x"}, Files: nil, Embeds: nil},
			m:    &Manifest{Source: "github.com/wakuwaku3/flame", Version: "v1.1.0", Ignore: nil, Stage1ExtraAgents: nil},
			want: true,
		},
		{
			name: "source が異なれば true",
			prev: &Lock{Installed: &LockInstalled{Source: "github.com/old/flame", Version: "v1.0.0", TreeHash: "sha256:x"}, Files: nil, Embeds: nil},
			m:    &Manifest{Source: "github.com/wakuwaku3/flame", Version: "v1.0.0", Ignore: nil, Stage1ExtraAgents: nil},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := needsRefetch(tc.prev, tc.m)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMakeVendorWritable(t *testing.T) {
	t.Parallel()

	t.Run("readonly 化された tree を 644/755 に戻せる", func(t *testing.T) {
		t.Parallel()
		root := buildVendorTree(t)
		require.NoError(t, makeVendorReadOnly(context.Background(), root))

		require.NoError(t, makeVendorWritable(context.Background(), root))

		assertPerm(t, root, dirPerm)
		assertPerm(t, filepath.Join(root, ".claude"), dirPerm)
		assertPerm(t, filepath.Join(root, "CLAUDE.md"), filePerm)
		require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("after"), fsperm.File))
	})
}

func TestNormalizeGitURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "https はそのまま", in: "https://github.com/wakuwaku3/flame", want: "https://github.com/wakuwaku3/flame"},
		{name: "git@ はそのまま (SSH)", in: "git@github.com:wakuwaku3/flame.git", want: "git@github.com:wakuwaku3/flame.git"},
		{name: "github.com/owner/repo は https:// prefix を補完", in: "github.com/wakuwaku3/flame", want: "https://github.com/wakuwaku3/flame"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, normalizeGitURL(tc.in))
		})
	}
}

func TestCopyTree(t *testing.T) {
	t.Parallel()

	t.Run("file 内容と dir 構造が保存される", func(t *testing.T) {
		t.Parallel()
		src := t.TempDir()
		dst := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), fsperm.Dir))
		require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha"), fsperm.File))
		require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("bravo"), fsperm.File))
		require.NoError(t, os.WriteFile(filepath.Join(src, "exec.sh"), []byte("#!/bin/sh\n"), fsperm.Exec))

		require.NoError(t, copyTree(context.Background(), src, dst))

		assertPerm(t, filepath.Join(dst, "a.txt"), filePerm)
		assert.Equal(t, "alpha", string(mustReadFile(t, filepath.Join(dst, "a.txt"))))
		assert.Equal(t, "bravo", string(mustReadFile(t, filepath.Join(dst, "sub", "b.txt"))))
		info, err := os.Stat(filepath.Join(dst, "exec.sh"))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&os.FileMode(0o111))
	})
}

func mustReadFile(tb testing.TB, path string) []byte {
	tb.Helper()
	data, err := os.ReadFile(path)
	require.NoError(tb, err)
	return data
}

func buildVendorTree(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	require.NoError(tb, os.MkdirAll(filepath.Join(root, ".claude", "rules"), fsperm.Dir))
	require.NoError(tb, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("vendor claude\n"), fsperm.File))
	require.NoError(tb, os.WriteFile(filepath.Join(root, ".claude", "rules", "x.md"), []byte("rule\n"), fsperm.File))
	return root
}

func assertPerm(tb testing.TB, path string, expected os.FileMode) {
	tb.Helper()
	info, err := os.Stat(path)
	require.NoError(tb, err)
	assert.Equal(tb, expected, info.Mode().Perm(), "perm mismatch for %s", path)
}
