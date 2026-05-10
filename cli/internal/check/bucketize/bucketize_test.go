package bucketize_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/check/bucketize"
)

// service-level test (FLM_APP_0009): Bucketize は file 群を content type 別の checker bucket に振り分ける classifier (= shell scripts/detect.sh と等価)。 各 content type の振り分け / Go module 配下の package 列挙 / 1 file 多 bucket 該当 (devbox.json) / 空入力 を service-level でカバーする。
//
//nolint:paralleltest // t.TempDir + filepath 解決と go module fixture を使うため parallel 不可
func TestBucketize(t *testing.T) {
	t.Run("各 content type ファイルは対応 checker bucket に分類される", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "docs/x.md", "")
		writeFile(t, repo, "scripts/foo.sh", "")
		writeFile(t, repo, "config/x.json", "")
		writeFile(t, repo, "config/y.yaml", "")
		writeFile(t, repo, "config/z.yml", "")
		files := []string{"docs/x.md", "scripts/foo.sh", "config/x.json", "config/y.yaml", "config/z.yml"}

		// Act
		entries, err := bucketize.Bucketize(repo, files)

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.Equal(t, []string{"docs/x.md"}, got[bucketize.CheckerDocument])
		assert.Equal(t, []string{"scripts/foo.sh"}, got[bucketize.CheckerShell])
		assert.Equal(t, []string{"config/x.json"}, got[bucketize.CheckerJSON])
		assert.Equal(t, []string{"config/y.yaml", "config/z.yml"}, got[bucketize.CheckerYAML])
	})

	t.Run("ADR 配下の md は check-document と check-adr の両方に入る", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "docs/adr/general/FLM_GEN_0099__x.md", "")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"docs/adr/general/FLM_GEN_0099__x.md"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.Equal(t, []string{"docs/adr/general/FLM_GEN_0099__x.md"}, got[bucketize.CheckerDocument])
		assert.Equal(t, []string{"docs/adr/general/FLM_GEN_0099__x.md"}, got[bucketize.CheckerADR])
	})

	t.Run("devbox.json は check-json と check-devbox の両方に入る", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "devbox.json", "{}")
		writeFile(t, repo, "devbox.lock", "{}")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"devbox.json", "devbox.lock"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.Equal(t, []string{"devbox.json"}, got[bucketize.CheckerJSON])
		assert.Equal(t, []string{"devbox.json", "devbox.lock"}, got[bucketize.CheckerDevbox])
	})

	t.Run("docs/notes 配下は check-flow-document に入る", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "docs/notes/feat_foo/index.md", "")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"docs/notes/feat_foo/index.md"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.Equal(t, []string{"docs/notes/feat_foo/index.md"}, got[bucketize.CheckerFlowDocument])
	})

	t.Run(".github/workflows の yaml は check-github-actions に入る", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, ".github/workflows/wf.yaml", "")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{".github/workflows/wf.yaml"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.Equal(t, []string{".github/workflows/wf.yaml"}, got[bucketize.CheckerGitHubActions])
	})

	t.Run("Go file は所属 module 内の全 package を lint target に含める", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "lib/go.mod", "module example.com/lib\ngo 1.26\n")
		writeFile(t, repo, "lib/clix/io.go", "package clix\n")
		writeFile(t, repo, "lib/ex/wrap.go", "package ex\n")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"lib/clix/io.go"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.ElementsMatch(t, []string{"lib/clix", "lib/ex"}, got[bucketize.CheckerGoLint])
	})

	t.Run("main package は check-go-build に追加で入る", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "cli/go.mod", "module example.com/cli\ngo 1.26\n")
		writeFile(t, repo, "cli/cmd/flame/main.go", "package main\n\nfunc main() {}\n")
		writeFile(t, repo, "cli/internal/util/util.go", "package util\n")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"cli/cmd/flame/main.go"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.ElementsMatch(t, []string{"cli/cmd/flame", "cli/internal/util"}, got[bucketize.CheckerGoLint])
		assert.Equal(t, []string{"cli/cmd/flame"}, got[bucketize.CheckerGoBuild])
	})

	t.Run("test ファイルを含む package は check-go-test に追加で入る", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "lib/go.mod", "module example.com/lib\ngo 1.26\n")
		writeFile(t, repo, "lib/foo/foo.go", "package foo\n")
		writeFile(t, repo, "lib/foo/foo_test.go", "package foo\n")
		writeFile(t, repo, "lib/bar/bar.go", "package bar\n")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"lib/foo/foo.go"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.ElementsMatch(t, []string{"lib/foo", "lib/bar"}, got[bucketize.CheckerGoLint])
		assert.Equal(t, []string{"lib/foo"}, got[bucketize.CheckerGoTest])
	})

	t.Run("vendor / .git / .devbox / .direnv 配下は package enumeration から prune される", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "lib/go.mod", "module example.com/lib\ngo 1.26\n")
		writeFile(t, repo, "lib/clix/io.go", "package clix\n")
		writeFile(t, repo, "lib/vendor/v/v.go", "package v\n")
		writeFile(t, repo, "lib/.devbox/d/d.go", "package d\n")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"lib/clix/io.go"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.Equal(t, []string{"lib/clix"}, got[bucketize.CheckerGoLint], "vendor / .devbox 配下は除外される")
	})

	t.Run("_test.go の package main は build target に入らない", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "cli/go.mod", "module example.com/cli\ngo 1.26\n")
		// 本ファイルは production code としては存在しないが、 test ファイルの `package main` (黒箱 test の慣習) を build target 候補に入れない invariant を保証する。
		writeFile(t, repo, "cli/internal/util/util.go", "package util\n")
		writeFile(t, repo, "cli/internal/util/util_main_test.go", "package main\n")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"cli/internal/util/util.go"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.NotContains(t, got, bucketize.CheckerGoBuild, "_test.go の package main は build target に入らない")
		assert.Equal(t, []string{"cli/internal/util"}, got[bucketize.CheckerGoTest])
	})

	t.Run("複数 module 配下のファイルはそれぞれ全 package が enumerate される", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "lib/go.mod", "module example.com/lib\ngo 1.26\n")
		writeFile(t, repo, "lib/clix/io.go", "package clix\n")
		writeFile(t, repo, "cli/go.mod", "module example.com/cli\ngo 1.26\n")
		writeFile(t, repo, "cli/internal/util/util.go", "package util\n")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"lib/clix/io.go", "cli/internal/util/util.go"})

		// Assert
		require.NoError(t, err)
		got := bucketsByChecker(entries)
		assert.ElementsMatch(t, []string{"lib/clix", "cli/internal/util"}, got[bucketize.CheckerGoLint])
	})

	t.Run("空 input は空 list を返す", func(t *testing.T) {
		// Act
		entries, err := bucketize.Bucketize(t.TempDir(), nil)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("entry は checker 名で sort 済", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeFile(t, repo, "x.json", "{}")
		writeFile(t, repo, "x.sh", "")
		writeFile(t, repo, "x.md", "")

		// Act
		entries, err := bucketize.Bucketize(repo, []string{"x.json", "x.sh", "x.md"})

		// Assert
		require.NoError(t, err)
		require.Len(t, entries, 3)
		assert.Equal(t, bucketize.CheckerDocument, entries[0].Checker)
		assert.Equal(t, bucketize.CheckerJSON, entries[1].Checker)
		assert.Equal(t, bucketize.CheckerShell, entries[2].Checker)
	})
}

func setupRepo(tb testing.TB) string {
	tb.Helper()
	return tb.TempDir()
}

func writeFile(tb testing.TB, repo, rel, body string) {
	tb.Helper()
	full := filepath.Join(repo, rel)
	require.NoError(tb, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(tb, os.WriteFile(full, []byte(body), 0o600))
}

func bucketsByChecker(entries []bucketize.Entry) map[string][]string {
	out := make(map[string][]string, len(entries))
	for _, e := range entries {
		out[e.Checker] = e.Targets
	}
	return out
}
