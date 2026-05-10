package install

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	t.Run("merge=replace は overlay 不在で vendor をそのまま返す", func(t *testing.T) {
		t.Parallel()
		out, err := Merge(&MergeInput{
			Strategy:       MergeReplace,
			VendorContent:  []byte("vendor only\n"),
			OverlayContent: nil,
			InstallPath:    "out.bin",
			ArrayStrategy:  "",
		})
		require.NoError(t, err)
		assert.Equal(t, "vendor only\n", string(out))
	})

	t.Run("merge=replace は overlay 提示で error", func(t *testing.T) {
		t.Parallel()
		_, err := Merge(&MergeInput{
			Strategy:       MergeReplace,
			VendorContent:  []byte("v"),
			OverlayContent: []byte("o"),
			InstallPath:    "out.bin",
			ArrayStrategy:  "",
		})
		require.Error(t, err)
	})

	t.Run("merge=append は vendor + overlay を改行区切りで連結", func(t *testing.T) {
		t.Parallel()
		out, err := Merge(&MergeInput{
			Strategy:       MergeAppend,
			VendorContent:  []byte("base\n"),
			OverlayContent: []byte("extra\n"),
			InstallPath:    ".shellcheckrc",
			ArrayStrategy:  "",
		})
		require.NoError(t, err)
		assert.Equal(t, "base\nextra\n", string(out))
	})

	t.Run("merge=append (markdown) は空行を入れて連結", func(t *testing.T) {
		t.Parallel()
		out, err := Merge(&MergeInput{
			Strategy:       MergeAppend,
			VendorContent:  []byte("# Title\nbody\n"),
			OverlayContent: []byte("## Extra\n"),
			InstallPath:    "doc.md",
			ArrayStrategy:  "",
		})
		require.NoError(t, err)
		// markdown は \n 区切り (vendor 末尾改行アリの場合)
		assert.Equal(t, "# Title\nbody\n\n## Extra\n", string(out))
	})

	t.Run("merge=deep (yaml) は mapping を再帰的に統合する", func(t *testing.T) {
		t.Parallel()
		vendor := []byte("a: 1\nb:\n  x: 10\n  y: 20\n")
		overlay := []byte("a: 99\nb:\n  y: 21\n  z: 30\nc: 3\n")
		out, err := Merge(&MergeInput{
			Strategy:       MergeDeep,
			VendorContent:  vendor,
			OverlayContent: overlay,
			InstallPath:    "config.yaml",
			ArrayStrategy:  "",
		})
		require.NoError(t, err)
		s := string(out)
		assert.Contains(t, s, "a: 99")
		assert.Contains(t, s, "x: 10")
		assert.Contains(t, s, "y: 21")
		assert.Contains(t, s, "z: 30")
		assert.Contains(t, s, "c: 3")
	})

	t.Run("merge=deep (yaml) sequence default は append", func(t *testing.T) {
		t.Parallel()
		vendor := []byte("items:\n  - a\n  - b\n")
		overlay := []byte("items:\n  - c\n")
		out, err := Merge(&MergeInput{
			Strategy:       MergeDeep,
			VendorContent:  vendor,
			OverlayContent: overlay,
			InstallPath:    "list.yaml",
			ArrayStrategy:  "",
		})
		require.NoError(t, err)
		s := string(out)
		// 順序が保たれて a, b, c が含まれる
		assert.Less(t, strings.Index(s, "a"), strings.Index(s, "c"))
		assert.Contains(t, s, "- c")
	})

	t.Run("merge=deep (yaml) sequence replace は overlay で全置換", func(t *testing.T) {
		t.Parallel()
		vendor := []byte("items:\n  - a\n  - b\n")
		overlay := []byte("items:\n  - c\n")
		out, err := Merge(&MergeInput{
			Strategy:       MergeDeep,
			VendorContent:  vendor,
			OverlayContent: overlay,
			InstallPath:    "list.yaml",
			ArrayStrategy:  MergeArrayReplace,
		})
		require.NoError(t, err)
		s := string(out)
		assert.Contains(t, s, "- c")
		assert.NotContains(t, s, "- a")
	})

	t.Run("merge=deep (json) は mapping を再帰的に統合する", func(t *testing.T) {
		t.Parallel()
		vendor := []byte(`{"a": 1, "b": {"x": 10}}`)
		overlay := []byte(`{"b": {"y": 20}, "c": 3}`)
		out, err := Merge(&MergeInput{
			Strategy:       MergeDeep,
			VendorContent:  vendor,
			OverlayContent: overlay,
			InstallPath:    "settings.json",
			ArrayStrategy:  "",
		})
		require.NoError(t, err)
		s := string(out)
		assert.Contains(t, s, `"x": 10`)
		assert.Contains(t, s, `"y": 20`)
		assert.Contains(t, s, `"c": 3`)
	})
}
