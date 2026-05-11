package install_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/root/install"
)

func TestLoadManifest(t *testing.T) {
	t.Parallel()

	t.Run("最小 schema を読み込める", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeManifest(t, root, "---\nflame:\n  source: github.com/wakuwaku3/flame\n  version: v1.0.0\n")

		m, err := install.LoadManifest(context.Background(), root)
		require.NoError(t, err)
		assert.Equal(t, "github.com/wakuwaku3/flame", m.Source)
		assert.Equal(t, "v1.0.0", m.Version)
		assert.False(t, m.IsSelf())
		assert.Empty(t, m.Ignore)
	})

	t.Run("self version で IsSelf() true", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeManifest(t, root, "---\nflame:\n  source: github.com/wakuwaku3/flame\n  version: self\n")

		m, err := install.LoadManifest(context.Background(), root)
		require.NoError(t, err)
		assert.True(t, m.IsSelf())
	})

	t.Run("既知 ignore は受理される", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeManifest(t, root, "---\nflame:\n  source: github.com/wakuwaku3/flame\n  version: v1.0.0\n  ignore:\n    - gitignore\n    - claude/plugins\n    - vendor-sync\n")

		m, err := install.LoadManifest(context.Background(), root)
		require.NoError(t, err)
		assert.True(t, m.IsIgnored(install.FeatureGitignore))
		assert.True(t, m.IsIgnored(install.FeatureClaudePlugins))
		assert.True(t, m.IsIgnored(install.FeatureVendorSync))
		assert.False(t, m.IsIgnored(install.FeatureMarkdownLint))
	})

	t.Run("未知 ignore は error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeManifest(t, root, "---\nflame:\n  source: github.com/wakuwaku3/flame\n  version: v1.0.0\n  ignore:\n    - bogus-feature\n")

		_, err := install.LoadManifest(context.Background(), root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bogus-feature")
		assert.Contains(t, err.Error(), "unknown ignore feature")
	})

	t.Run("旧 ignore 命名 (.gitignore) は未知扱いで error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeManifest(t, root, "---\nflame:\n  source: github.com/wakuwaku3/flame\n  version: v1.0.0\n  ignore:\n    - .gitignore\n")

		_, err := install.LoadManifest(context.Background(), root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ".gitignore")
	})

	t.Run("source 欠如は error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeManifest(t, root, "---\nflame:\n  version: v1.0.0\n")

		_, err := install.LoadManifest(context.Background(), root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source")
	})

	t.Run("version 欠如は error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeManifest(t, root, "---\nflame:\n  source: github.com/wakuwaku3/flame\n")

		_, err := install.LoadManifest(context.Background(), root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version")
	})

	t.Run("flame.yaml 不在は error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		_, err := install.LoadManifest(context.Background(), root)
		require.Error(t, err)
	})
}

func TestManifest_IsIgnored_EmptyFeature(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeManifest(t, root, "---\nflame:\n  source: github.com/wakuwaku3/flame\n  version: v1.0.0\n  ignore:\n    - gitignore\n")

	m, err := install.LoadManifest(context.Background(), root)
	require.NoError(t, err)
	// path から featureForInstall が "" を返すケースで誤って ignore 扱いしないこと
	assert.False(t, m.IsIgnored(""))
}

func writeManifest(tb testing.TB, root, body string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(filepath.Join(root, "flame.yaml"), []byte(body), fsperm.File))
}
