package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
)

func TestApplyPluginMarketplace(t *testing.T) {
	t.Parallel()

	t.Run("settings.json 不在で新規作成される", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		require.NoError(t, applyPluginMarketplace(context.Background(), root, "github.com/wakuwaku3/flame"))

		settings := mustReadJSON(t, filepath.Join(root, ".claude", "settings.json"))
		marketplaces, ok := settings["extraKnownMarketplaces"].(map[string]any)
		require.True(t, ok)
		require.NotNil(t, marketplaces["flame"])
		enabled, ok := settings["enabledPlugins"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, enabled["flame@flame"])
	})

	t.Run("既存 settings.json に追記する (他キーを保持)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), fsperm.Dir))
		original := `{"theme":"dark","extraKnownMarketplaces":{"other":{"source":{"source":"github","repo":"x/y"}}}}`
		require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(original), fsperm.File))

		require.NoError(t, applyPluginMarketplace(context.Background(), root, "github.com/wakuwaku3/flame"))

		settings := mustReadJSON(t, filepath.Join(root, ".claude", "settings.json"))
		assert.Equal(t, "dark", settings["theme"])
		marketplaces, ok := settings["extraKnownMarketplaces"].(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, marketplaces["other"])
		assert.NotNil(t, marketplaces["flame"])
	})

	t.Run("source 形式不正は error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		err := applyPluginMarketplace(context.Background(), root, "invalid")
		require.Error(t, err)
	})

	t.Run("extraKnownMarketplaces が object でなければ error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), fsperm.Dir))
		broken := `{"extraKnownMarketplaces":"not-an-object"}`
		require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(broken), fsperm.File))

		err := applyPluginMarketplace(context.Background(), root, "github.com/wakuwaku3/flame")
		require.Error(t, err)
	})
}

func mustReadJSON(tb testing.TB, path string) map[string]any {
	tb.Helper()
	data, err := os.ReadFile(path)
	require.NoError(tb, err)
	var out map[string]any
	require.NoError(tb, json.Unmarshal(data, &out))
	return out
}
