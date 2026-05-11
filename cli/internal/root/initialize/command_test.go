package initialize_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/initialize"
)

// TestRun は flame init endpoint の service-level test (FLM_APP_0009)。 cwd 固定 / 既存 flame.yaml の上書き禁止 / 対話モード / -y / --source / --version flag / default version 整形の各経路を検証する。
func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("-y で default 採用: release binary version は v1.2.3 形式で書き込まれる", func(t *testing.T) {
		t.Parallel()

		// Arrange
		root := t.TempDir()
		stdout := &strings.Builder{}
		stderr := &strings.Builder{}
		opts := &initialize.RunOptions{
			RepoRoot:        root,
			BinaryVersion:   "1.2.3",
			Yes:             true,
			SourceOverride:  "",
			VersionOverride: "",
			Stdin:           strings.NewReader(""),
			Stdout:          stdout,
			Stderr:          stderr,
		}

		// Act
		err := initialize.Run(context.Background(), opts)

		// Assert
		require.NoError(t, err)
		manifest, readErr := os.ReadFile(filepath.Join(root, "flame.yaml"))
		require.NoError(t, readErr)
		expected := "---\n" +
			"# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml\n" +
			"flame:\n" +
			"  source: github.com/wakuwaku3/flame\n" +
			"  version: v1.2.3\n"
		assert.Equal(t, expected, string(manifest))
		assert.Equal(t, "flame init: wrote "+filepath.Join(root, "flame.yaml")+" (source=github.com/wakuwaku3/flame, version=v1.2.3)\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("-y + dev binary version は placeholder v0.0.0 にフォールバック", func(t *testing.T) {
		t.Parallel()

		// Arrange
		root := t.TempDir()
		opts := &initialize.RunOptions{
			RepoRoot:        root,
			BinaryVersion:   "0.0.0-dev",
			Yes:             true,
			SourceOverride:  "",
			VersionOverride: "",
			Stdin:           strings.NewReader(""),
			Stdout:          &strings.Builder{},
			Stderr:          &strings.Builder{},
		}

		// Act
		err := initialize.Run(context.Background(), opts)

		// Assert
		require.NoError(t, err)
		manifest, readErr := os.ReadFile(filepath.Join(root, "flame.yaml"))
		require.NoError(t, readErr)
		expected := "---\n" +
			"# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml\n" +
			"flame:\n" +
			"  source: github.com/wakuwaku3/flame\n" +
			"  version: v0.0.0\n"
		assert.Equal(t, expected, string(manifest))
	})

	t.Run("対話モード: Enter 連打で default 採用、 入力ありで override", func(t *testing.T) {
		t.Parallel()

		// Arrange
		root := t.TempDir()
		stdout := &strings.Builder{}
		opts := &initialize.RunOptions{
			RepoRoot:        root,
			BinaryVersion:   "1.0.0",
			Yes:             false,
			SourceOverride:  "",
			VersionOverride: "",
			Stdin:           strings.NewReader("\nv2.0.0\n"),
			Stdout:          stdout,
			Stderr:          &strings.Builder{},
		}

		// Act
		err := initialize.Run(context.Background(), opts)

		// Assert
		require.NoError(t, err)
		manifest, readErr := os.ReadFile(filepath.Join(root, "flame.yaml"))
		require.NoError(t, readErr)
		expected := "---\n" +
			"# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml\n" +
			"flame:\n" +
			"  source: github.com/wakuwaku3/flame\n" +
			"  version: v2.0.0\n"
		assert.Equal(t, expected, string(manifest))
		expectedStdout := "flame source: (github.com/wakuwaku3/flame) flame version: (v1.0.0) flame init: wrote " + filepath.Join(root, "flame.yaml") + " (source=github.com/wakuwaku3/flame, version=v2.0.0)\n"
		assert.Equal(t, expectedStdout, stdout.String())
	})

	t.Run("--source / --version flag は対話 prompt を skip して値を直接採用", func(t *testing.T) {
		t.Parallel()

		// Arrange
		root := t.TempDir()
		stdout := &strings.Builder{}
		opts := &initialize.RunOptions{
			RepoRoot:        root,
			BinaryVersion:   "1.0.0",
			Yes:             false,
			SourceOverride:  "github.com/example/fork",
			VersionOverride: "v3.0.0",
			Stdin:           strings.NewReader(""),
			Stdout:          stdout,
			Stderr:          &strings.Builder{},
		}

		// Act
		err := initialize.Run(context.Background(), opts)

		// Assert
		require.NoError(t, err)
		manifest, readErr := os.ReadFile(filepath.Join(root, "flame.yaml"))
		require.NoError(t, readErr)
		expected := "---\n" +
			"# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml\n" +
			"flame:\n" +
			"  source: github.com/example/fork\n" +
			"  version: v3.0.0\n"
		assert.Equal(t, expected, string(manifest))
		expectedStdout := "flame init: wrote " + filepath.Join(root, "flame.yaml") + " (source=github.com/example/fork, version=v3.0.0)\n"
		assert.Equal(t, expectedStdout, stdout.String())
	})

	t.Run("既存 flame.yaml が cwd 直下にあれば上書きせず error", func(t *testing.T) {
		t.Parallel()

		// Arrange
		root := t.TempDir()
		preExisting := "flame:\n  source: github.com/user/repo\n  version: v9.9.9\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, "flame.yaml"), []byte(preExisting), 0o600))
		opts := &initialize.RunOptions{
			RepoRoot:        root,
			BinaryVersion:   "1.0.0",
			Yes:             true,
			SourceOverride:  "",
			VersionOverride: "",
			Stdin:           strings.NewReader(""),
			Stdout:          &strings.Builder{},
			Stderr:          &strings.Builder{},
		}

		// Act
		err := initialize.Run(context.Background(), opts)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "flame.yaml already exists")
		after, readErr := os.ReadFile(filepath.Join(root, "flame.yaml"))
		require.NoError(t, readErr)
		assert.Equal(t, preExisting, string(after))
	})

	t.Run("binary version が既に v 接頭辞付きならそのまま採用", func(t *testing.T) {
		t.Parallel()

		// Arrange
		root := t.TempDir()
		opts := &initialize.RunOptions{
			RepoRoot:        root,
			BinaryVersion:   "v2.5.1",
			Yes:             true,
			SourceOverride:  "",
			VersionOverride: "",
			Stdin:           strings.NewReader(""),
			Stdout:          &strings.Builder{},
			Stderr:          &strings.Builder{},
		}

		// Act
		err := initialize.Run(context.Background(), opts)

		// Assert
		require.NoError(t, err)
		manifest, readErr := os.ReadFile(filepath.Join(root, "flame.yaml"))
		require.NoError(t, readErr)
		expected := "---\n" +
			"# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml\n" +
			"flame:\n" +
			"  source: github.com/wakuwaku3/flame\n" +
			"  version: v2.5.1\n"
		assert.Equal(t, expected, string(manifest))
	})
}
