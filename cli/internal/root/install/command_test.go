package install_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/root/install"
)

const (
	testFilePerm os.FileMode = 0o644
	testReadOnly os.FileMode = 0o444
)

// TestRun は flame install の service-level test (FLM_APP_0009)。 fixture repo を tmp dir に組み立て、 install.Run を直接呼んで vendor SoT が install path に同期されること、 flame.lock が想定通り生成されること、 chmod 444 が効くこと、 .gitignore / .claude/settings.json が更新されることを検証する。
func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("downstream version: vendor sync + lock 生成 + chmod 444 + .gitignore + plugin settings", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		writeFixtureRepo(t, root, "v1.0.0")

		// Act
		err := install.Run(context.Background(), root, os.Stdout, os.Stderr)

		// Assert
		require.NoError(t, err)

		// install copy が vendor 内容で配置される
		assertFileContains(t, filepath.Join(root, ".golangci.yaml"), "linters:\n")
		assertFilePerm(t, filepath.Join(root, ".golangci.yaml"), testReadOnly)

		// .shellcheckrc は merge: append (overlay 無いので vendor 内容のみ)
		assertFileEquals(t, filepath.Join(root, ".shellcheckrc"), "disable=SC2016\n")

		// embed snippet が CLAUDE.md に scaffold される
		assertFileContains(t, filepath.Join(root, "CLAUDE.md"), "[vendor/flame/CLAUDE.md](vendor/flame/CLAUDE.md)")
		// embed file は read-only 化対象外
		assertFilePerm(t, filepath.Join(root, "CLAUDE.md"), testFilePerm)

		// flame.lock 生成
		lockBytes := mustRead(t, filepath.Join(root, "flame.lock"))
		assert.Contains(t, string(lockBytes), "install: .golangci.yaml")
		assert.Contains(t, string(lockBytes), "merge: deep")
		assert.Contains(t, string(lockBytes), "install: CLAUDE.md")

		// .gitignore に vendor block 追加
		gitignore := mustRead(t, filepath.Join(root, ".gitignore"))
		assert.Contains(t, string(gitignore), "vendor/*")

		// .claude/settings.json に plugin 登録
		settings := mustRead(t, filepath.Join(root, ".claude", "settings.json"))
		assert.Contains(t, string(settings), `"flame@flame": true`)
		assert.Contains(t, string(settings), `"extraKnownMarketplaces"`)
	})

	t.Run("self version: .gitignore / plugin install を skip", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		writeFixtureRepo(t, root, "self")
		// flame self の特例: ignore で skip
		writeFlameYAML(t, root, "self", []string{"gitignore", "claude/plugins"})

		// Act
		err := install.Run(context.Background(), root, os.Stdout, os.Stderr)

		// Assert
		require.NoError(t, err)
		// .gitignore は scaffold されない (元から無いので存在しないことを確認)
		_, statErr := os.Stat(filepath.Join(root, ".gitignore"))
		assert.True(t, os.IsNotExist(statErr), ".gitignore should be skipped: %v", statErr)
		// .claude/settings.json も生成されない
		_, statErr = os.Stat(filepath.Join(root, ".claude", "settings.json"))
		assert.True(t, os.IsNotExist(statErr), ".claude/settings.json should be skipped: %v", statErr)
	})

	t.Run("downstream で uses ref が version で pin される", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		writeFixtureRepo(t, root, "v2.5.0")

		// Act
		err := install.Run(context.Background(), root, os.Stdout, os.Stderr)
		require.NoError(t, err)

		// Assert: trg__push__main → flame-trg__push__main で uses ref が @v2.5.0
		trgPath := filepath.Join(root, ".github", "workflows", "flame-trg__push__main.yaml")
		trg := mustRead(t, trgPath)
		assert.Contains(t, string(trg), "wakuwaku3/flame/.github/workflows/wf__deploy.yaml@v2.5.0")
	})

	t.Run("再 install は idempotent (file content 同一)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFixtureRepo(t, root, "v1.0.0")

		require.NoError(t, install.Run(context.Background(), root, os.Stdout, os.Stderr))
		first := mustRead(t, filepath.Join(root, "flame.lock"))

		require.NoError(t, install.Run(context.Background(), root, os.Stdout, os.Stderr))
		second := mustRead(t, filepath.Join(root, "flame.lock"))

		assert.Equal(t, string(first), string(second))
	})

	t.Run("flame.yaml が無い repo root では error (上方向探索しない)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		err := install.Run(context.Background(), root, os.Stdout, os.Stderr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "flame.yaml")
	})

	t.Run("overlay 経由で利用者拡張が install 結果に反映される (vendor unchanged)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFixtureRepo(t, root, "self")
		writeFlameYAML(t, root, "self", []string{"gitignore", "claude/plugins", "vendor-sync", "vendor-readonly"})
		// overlay = 「最終形」 で .shellcheckrc を書く (vendor base + 利用者拡張)
		mustWrite(t, filepath.Join(root, ".shellcheckrc.flame-overlay"), "disable=SC2016\ndisable=SC2086\n")
		require.NoError(t, install.Run(context.Background(), root, os.Stdout, os.Stderr))
		// 1 回目は base が無く overlay = 最終形採用
		got := mustRead(t, filepath.Join(root, ".shellcheckrc"))
		assert.Equal(t, "disable=SC2016\ndisable=SC2086\n", string(got))
	})

	t.Run("ignore directive で個別 feature の install copy が skip される", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFixtureRepo(t, root, "self")
		writeFlameYAML(t, root, "self", []string{"gitignore", "claude/plugins", "vendor-sync", "vendor-readonly", "shellcheck"})
		require.NoError(t, install.Run(context.Background(), root, os.Stdout, os.Stderr))
		// shellcheck feature 全体が skip → .shellcheckrc は install されない
		_, err := os.Stat(filepath.Join(root, ".shellcheckrc"))
		assert.True(t, os.IsNotExist(err), ".shellcheckrc should be skipped: %v", err)
		// 一方 .golangci.yaml は引き続き install される
		_, err = os.Stat(filepath.Join(root, ".golangci.yaml"))
		require.NoError(t, err)
	})

	t.Run("3-way merge で vendor 削除 + overlay kept が conflict として install を中断する", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFixtureRepo(t, root, "self")
		writeFlameYAML(t, root, "self", []string{"gitignore", "claude/plugins", "vendor-sync", "vendor-readonly"})
		// 初回 install: lock に base (= 現状の vendor) が記録される
		require.NoError(t, install.Run(context.Background(), root, os.Stdout, os.Stderr))
		// vendor から要素を削除 (= flame side が線を消した)
		mustWrite(t, filepath.Join(root, "vendor", "flame", ".shellcheckrc"), "")
		// overlay で消された行を kept
		mustWrite(t, filepath.Join(root, ".shellcheckrc.flame-overlay"), "disable=SC2016\n")

		err := install.Run(context.Background(), root, os.Stdout, os.Stderr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflict")
	})
}

// writeFixtureRepo は test 用に最小 repo 構造を組み立てる。 vendor/flame/ 配下に install 対象 file を配置し、 flame.yaml を repo root に置く。
func writeFixtureRepo(tb testing.TB, root, version string) {
	tb.Helper()
	mustMkdir(tb, filepath.Join(root, "vendor", "flame", ".github", "workflows"))
	mustMkdir(tb, filepath.Join(root, "vendor", "flame", ".claude", "rules"))
	mustWrite(tb, filepath.Join(root, "vendor", "flame", ".golangci.yaml"), "---\nlinters:\n  default: none\n  enable:\n    - errcheck\n")
	mustWrite(tb, filepath.Join(root, "vendor", "flame", ".shellcheckrc"), "disable=SC2016\n")
	mustWrite(tb, filepath.Join(root, "vendor", "flame", "CLAUDE.md"), "# vendor SoT CLAUDE\n")
	mustWrite(tb, filepath.Join(root, "vendor", "flame", ".envrc"), "export FLAME_VENDOR=1\n")
	mustWrite(tb, filepath.Join(root, "vendor", "flame", ".yamllint"), "extends: default\n")
	mustWrite(tb, filepath.Join(root, "vendor", "flame", ".github", "workflows", "trg__push__main.yaml"), "name: deploy\non:\n  push:\n    branches: [main]\njobs:\n  deploy:\n    uses: wakuwaku3/flame/.github/workflows/wf__deploy.yaml@main\n")
	writeFlameYAML(tb, root, version, nil)
}

func writeFlameYAML(tb testing.TB, root, version string, ignore []string) {
	tb.Helper()
	var b strings.Builder
	b.WriteString("---\nflame:\n  source: github.com/wakuwaku3/flame\n  version: ")
	b.WriteString(version)
	b.WriteString("\n")
	if len(ignore) > 0 {
		b.WriteString("  ignore:\n")
		for _, ig := range ignore {
			b.WriteString("    - ")
			b.WriteString(ig)
			b.WriteString("\n")
		}
	}
	mustWrite(tb, filepath.Join(root, "flame.yaml"), b.String())
}

func mustMkdir(tb testing.TB, path string) {
	tb.Helper()
	require.NoError(tb, os.MkdirAll(path, fsperm.Dir))
}

func mustWrite(tb testing.TB, path, content string) {
	tb.Helper()
	mustMkdir(tb, filepath.Dir(path))
	require.NoError(tb, os.WriteFile(path, []byte(content), fsperm.File))
}

func mustRead(tb testing.TB, path string) []byte {
	tb.Helper()
	data, err := os.ReadFile(path)
	require.NoError(tb, err)
	return data
}

func assertFileEquals(tb testing.TB, path, expected string) {
	tb.Helper()
	assert.Equal(tb, expected, string(mustRead(tb, path)))
}

func assertFileContains(tb testing.TB, path, substring string) {
	tb.Helper()
	assert.Contains(tb, string(mustRead(tb, path)), substring)
}

func assertFilePerm(tb testing.TB, path string, expected os.FileMode) {
	tb.Helper()
	info, err := os.Stat(path)
	require.NoError(tb, err)
	assert.Equal(tb, expected, info.Mode().Perm(), "perm mismatch for %s", path)
}
