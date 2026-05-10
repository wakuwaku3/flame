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

func TestLockRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("Installed セクション + files + embeds の write/read 整合", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		original := &install.Lock{
			Installed: &install.LockInstalled{
				Source:   "github.com/wakuwaku3/flame",
				Version:  "v1.2.3",
				TreeHash: "sha256:abcdef",
			},
			Files: []install.LockFile{
				{
					Install:       ".golangci.yaml",
					Vendor:        "vendor/flame/.golangci.yaml",
					Merge:         install.MergeDeep,
					Content:       "linters: {}\n",
					VendorContent: "linters: {}\n",
					Overlay: &install.LockOverlay{
						Path:    ".golangci.flame-overlay.yaml",
						Content: "linters: {disable: []}\n",
					},
					MergeArray: "",
				},
			},
			Embeds: []install.LockEmbed{
				{Install: "CLAUDE.md", Target: "vendor/flame/CLAUDE.md", Snippet: "[vendor/flame/CLAUDE.md](vendor/flame/CLAUDE.md)\n"},
			},
		}

		require.NoError(t, install.WriteLock(context.Background(), root, original))
		got, err := install.LoadLock(context.Background(), root)
		require.NoError(t, err)

		require.NotNil(t, got.Installed)
		assert.Equal(t, original.Installed.Source, got.Installed.Source)
		assert.Equal(t, original.Installed.Version, got.Installed.Version)
		assert.Equal(t, original.Installed.TreeHash, got.Installed.TreeHash)

		require.Len(t, got.Files, 1)
		assert.Equal(t, original.Files[0].Install, got.Files[0].Install)
		assert.Equal(t, original.Files[0].VendorContent, got.Files[0].VendorContent)
		require.NotNil(t, got.Files[0].Overlay)
		assert.Equal(t, original.Files[0].Overlay.Content, got.Files[0].Overlay.Content)

		require.Len(t, got.Embeds, 1)
		assert.Equal(t, original.Embeds[0].Snippet, got.Embeds[0].Snippet)
	})

	t.Run("Installed.TreeHash 空 (self mode) は YAML に出力しない", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		lock := &install.Lock{
			Installed: &install.LockInstalled{Source: "github.com/x/y", Version: "self", TreeHash: ""},
			Files:     nil,
			Embeds:    nil,
		}
		require.NoError(t, install.WriteLock(context.Background(), root, lock))
		body, err := os.ReadFile(filepath.Join(root, "flame.lock"))
		require.NoError(t, err)
		assert.NotContains(t, string(body), "tree_hash")
		assert.Contains(t, string(body), "version: self")
	})

	t.Run("flame.lock 不在は空 Lock を返す (初回 install)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		lock, err := install.LoadLock(context.Background(), root)
		require.NoError(t, err)
		assert.Nil(t, lock.Installed)
		assert.Empty(t, lock.Files)
		assert.Empty(t, lock.Embeds)
	})

	t.Run("旧 schema (flame.harness.X) は新フィールドにマップされず空扱いになる", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		old := "---\nflame:\n  harness:\n    files:\n      - install: .x\n        vendor: vendor/flame/.x\n        merge: append\n        content: legacy\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, "flame.lock"), []byte(old), fsperm.File))
		got, err := install.LoadLock(context.Background(), root)
		require.NoError(t, err)
		assert.Empty(t, got.Files)
		assert.Empty(t, got.Embeds)
	})
}
