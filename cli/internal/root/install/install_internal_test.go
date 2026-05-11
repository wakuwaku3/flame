package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverlayPathFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		installPath string
		want        string
	}{
		{name: "拡張子のあるファイルは <stem>.flame-overlay.<ext>", installPath: ".golangci.yaml", want: ".golangci.flame-overlay.yaml"},
		{name: "通常 file (devbox.json) は基本パターン", installPath: "devbox.json", want: "devbox.flame-overlay.json"},
		{name: "hidden file (.shellcheckrc, 単一 dot 始まり) は no-ext 扱い", installPath: ".shellcheckrc", want: ".shellcheckrc.flame-overlay"},
		{name: ".envrc も hidden file 扱い", installPath: ".envrc", want: ".envrc.flame-overlay"},
		{name: "サブディレクトリ配下も同じ規則", installPath: ".claude/rules/x.md", want: ".claude/rules/x.flame-overlay.md"},
		{name: "拡張子なしの通常 file", installPath: "Makefile", want: "Makefile.flame-overlay"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Act
			got := overlayPathFor(tc.installPath)
			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWriteConflictMarker(t *testing.T) {
	t.Parallel()

	t.Run("trailing newline 付きの overlay/vendor: 既存改行を尊重して marker を挿入", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		conflicts := []MergeConflict{{Path: "items[0]", Description: "their (vendor) removed array element; our (overlay) kept"}}
		// Act
		err := writeConflictMarker(root, ".shellcheckrc", []byte("our-line\n"), []byte("their-line\n"), conflicts)
		// Assert
		require.NoError(t, err)
		got, readErr := os.ReadFile(filepath.Join(root, ".shellcheckrc.flame-overlay"))
		require.NoError(t, readErr)
		expected := "# flame install: 3-way merge conflict (install=.shellcheckrc)\n" +
			"#   - path=items[0]: their (vendor) removed array element; our (overlay) kept\n" +
			"# Resolve by editing this file (remove markers and apply intended values), then re-run `flame install`.\n" +
			"<<<<<<< OURS (current overlay)\n" +
			"our-line\n" +
			"=======\n" +
			"their-line\n" +
			">>>>>>> THEIRS (current vendor)\n"
		assert.Equal(t, expected, string(got))
	})

	t.Run("trailing newline 無しの overlay/vendor: 改行補完して marker を挿入", func(t *testing.T) {
		t.Parallel()
		// Arrange
		root := t.TempDir()
		conflicts := []MergeConflict{{Path: "x", Description: "scalar conflict"}}
		// Act
		err := writeConflictMarker(root, ".shellcheckrc", []byte("our-no-newline"), []byte("their-no-newline"), conflicts)
		// Assert
		require.NoError(t, err)
		got, readErr := os.ReadFile(filepath.Join(root, ".shellcheckrc.flame-overlay"))
		require.NoError(t, readErr)
		expected := "# flame install: 3-way merge conflict (install=.shellcheckrc)\n" +
			"#   - path=x: scalar conflict\n" +
			"# Resolve by editing this file (remove markers and apply intended values), then re-run `flame install`.\n" +
			"<<<<<<< OURS (current overlay)\n" +
			"our-no-newline\n" +
			"=======\n" +
			"their-no-newline\n" +
			">>>>>>> THEIRS (current vendor)\n"
		assert.Equal(t, expected, string(got))
	})
}

func TestFormatConflictError(t *testing.T) {
	t.Parallel()

	t.Run("conflict 2 件を path + description で列挙し overlay 編集を促す文言を含む", func(t *testing.T) {
		t.Parallel()
		// Arrange
		conflicts := []MergeConflict{
			{Path: "linters.enable[0]", Description: "their (vendor) removed array element; our (overlay) kept"},
			{Path: "run.timeout", Description: "base=1m their=5m our=10m"},
		}
		// Act
		err := formatConflictError(".golangci.yaml", conflicts)
		// Assert
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, ".golangci.yaml")
		assert.Contains(t, msg, "2 conflict(s)")
		assert.Contains(t, msg, "linters.enable[0]")
		assert.Contains(t, msg, "run.timeout")
		assert.Contains(t, msg, "resolve by editing the overlay file")
	})
}
