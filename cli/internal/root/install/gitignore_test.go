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

func TestApplyGitignore(t *testing.T) {
	t.Parallel()

	const expectedBlock = "# flame harness が install 時に追加する block (FLM_FEA_0003)\ntmp\n.devbox\n.direnv\n.local\n.claude/.ccache\n.claude/scheduled_tasks.lock\nvendor/*\n"

	t.Run(".gitignore が無い repo では scaffold で新規作成される", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		path := filepath.Join(root, ".gitignore")

		// Act
		err := applyGitignore(context.Background(), root)

		// Assert
		require.NoError(t, err)
		got, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, expectedBlock, string(got))
	})

	t.Run("別内容の .gitignore に対しては末尾に block が追記される", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		path := filepath.Join(root, ".gitignore")
		original := "node_modules\ndist\n"
		require.NoError(t, os.WriteFile(path, []byte(original), fsperm.File))

		// Act
		err := applyGitignore(context.Background(), root)

		// Assert
		require.NoError(t, err)
		got, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, original+"\n"+expectedBlock, string(got))
	})

	t.Run("末尾改行なしの .gitignore に対しても改行付きで追記される", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		path := filepath.Join(root, ".gitignore")
		original := "node_modules\ndist"
		require.NoError(t, os.WriteFile(path, []byte(original), fsperm.File))

		// Act
		err := applyGitignore(context.Background(), root)

		// Assert
		require.NoError(t, err)
		got, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, original+"\n\n"+expectedBlock, string(got))
	})

	t.Run("vendor/* を既に含む .gitignore は no-op", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		path := filepath.Join(root, ".gitignore")
		original := "node_modules\nvendor/*\ndist\n"
		require.NoError(t, os.WriteFile(path, []byte(original), fsperm.File))

		// Act
		err := applyGitignore(context.Background(), root)

		// Assert
		require.NoError(t, err)
		got, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, original, string(got))
	})

	t.Run("連続 2 回呼んでも内容が同一 (idempotent)", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		path := filepath.Join(root, ".gitignore")

		// Act
		require.NoError(t, applyGitignore(context.Background(), root))
		first, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.NoError(t, applyGitignore(context.Background(), root))
		second, readErr := os.ReadFile(path)
		require.NoError(t, readErr)

		// Assert
		assert.Equal(t, string(first), string(second))
	})
}
