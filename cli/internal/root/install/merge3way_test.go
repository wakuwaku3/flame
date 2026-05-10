package install

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge3Way_YAML_OverlayOnly(t *testing.T) {
	t.Parallel()

	t.Run("overlay 不在は vendor をそのまま返す", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\n"),
			TheirContent: []byte("a: 2\n"),
			OurContent:   nil,
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Equal(t, "a: 2\n", string(out.Content))
	})

	t.Run("base 不在 (= 初回 install with overlay) は overlay をそのまま返す", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  nil,
			TheirContent: []byte("a: 1\n"),
			OurContent:   []byte("a: 99\nb: 2\n"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Equal(t, "a: 99\nb: 2\n", string(out.Content))
	})
}

func TestMerge3Way_YAML_Sequence(t *testing.T) {
	t.Parallel()

	t.Run("user が末尾追加 → 結果に保持される", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("items:\n  - a\n  - b\n"),
			TheirContent: []byte("items:\n  - a\n  - b\n"),
			OurContent:   []byte("items:\n  - a\n  - b\n  - c\n"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Contains(t, string(out.Content), "- c")
	})

	t.Run("vendor 追加 + user 追加が両立 (両方反映)", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("items:\n  - a\n  - b\n"),
			TheirContent: []byte("items:\n  - a\n  - b\n  - x\n"),
			OurContent:   []byte("items:\n  - a\n  - b\n  - c\n"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		s := string(out.Content)
		assert.Contains(t, s, "- x")
		assert.Contains(t, s, "- c")
	})

	t.Run("vendor 削除 + user kept → conflict", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("items:\n  - a\n  - b\n"),
			TheirContent: []byte("items:\n  - a\n"),
			OurContent:   []byte("items:\n  - a\n  - b\n"),
		})
		require.NoError(t, err)
		require.Len(t, out.Conflicts, 1)
		assert.Contains(t, out.Conflicts[0].Description, "their")
	})

	t.Run("user removed (vendor kept) → user の削除を尊重", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("items:\n  - a\n  - b\n"),
			TheirContent: []byte("items:\n  - a\n  - b\n"),
			OurContent:   []byte("items:\n  - a\n"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		s := string(out.Content)
		assert.Contains(t, s, "- a")
		assert.NotContains(t, s, "- b")
	})
}

func TestMerge3Way_YAML_Mapping(t *testing.T) {
	t.Parallel()

	t.Run("user 値変更を尊重", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\n"),
			TheirContent: []byte("a: 1\n"),
			OurContent:   []byte("a: 99\n"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Contains(t, string(out.Content), "a: 99")
	})

	t.Run("vendor 値変更を user kept (= 同じ base) → vendor を取り込む", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\n"),
			TheirContent: []byte("a: 2\n"),
			OurContent:   []byte("a: 1\n"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Contains(t, string(out.Content), "a: 2")
	})

	t.Run("scalar 双方変更 (異なる値) → conflict", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\n"),
			TheirContent: []byte("a: 2\n"),
			OurContent:   []byte("a: 99\n"),
		})
		require.NoError(t, err)
		require.Len(t, out.Conflicts, 1)
		assert.Equal(t, "a", out.Conflicts[0].Path)
	})
}

func TestMerge3Way_JSON(t *testing.T) {
	t.Parallel()

	t.Run("array で user 追加が保持される", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.json",
			BaseContent:  []byte(`{"items":["a","b"]}`),
			TheirContent: []byte(`{"items":["a","b","x"]}`),
			OurContent:   []byte(`{"items":["a","b","c"]}`),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		s := string(out.Content)
		assert.Contains(t, s, `"x"`)
		assert.Contains(t, s, `"c"`)
	})
}

func TestMerge3Way_AppendText(t *testing.T) {
	t.Parallel()

	t.Run("vendor 追加 + user 追加が両立", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeAppend,
			InstallPath:  ".shellcheckrc",
			BaseContent:  []byte("base1\nbase2\n"),
			TheirContent: []byte("base1\nbase2\nvendor-add\n"),
			OurContent:   []byte("base1\nbase2\nuser-add\n"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		s := string(out.Content)
		assert.Contains(t, s, "vendor-add")
		assert.Contains(t, s, "user-add")
	})

	t.Run("vendor 行削除 + user 同行 kept → conflict", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeAppend,
			InstallPath:  ".shellcheckrc",
			BaseContent:  []byte("base1\nbase2\n"),
			TheirContent: []byte("base1\n"),
			OurContent:   []byte("base1\nbase2\n"),
		})
		require.NoError(t, err)
		require.NotEmpty(t, out.Conflicts)
	})
}

func TestMerge3Way_Replace(t *testing.T) {
	t.Parallel()

	t.Run("replace は overlay 提示で error", func(t *testing.T) {
		t.Parallel()
		_, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeReplace,
			InstallPath:  "out.bin",
			BaseContent:  nil,
			TheirContent: []byte("v"),
			OurContent:   []byte("o"),
		})
		require.Error(t, err)
	})

	t.Run("replace は overlay 不在で vendor をそのまま返す", func(t *testing.T) {
		t.Parallel()
		out, err := Merge3Way(&Merge3WayInput{
			Strategy:     MergeReplace,
			InstallPath:  "out.bin",
			BaseContent:  nil,
			TheirContent: []byte("vendor only\n"),
			OurContent:   nil,
		})
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Equal(t, "vendor only\n", string(out.Content))
	})
}

func TestMerge3Way_NestedMapping(t *testing.T) {
	t.Parallel()

	out, err := Merge3Way(&Merge3WayInput{
		Strategy:    MergeDeep,
		InstallPath: "x.yaml",
		BaseContent: []byte("linters:\n  enable:\n    - errcheck\n    - govet\n"),
		TheirContent: []byte("linters:\n  enable:\n    - errcheck\n    - govet\n    - ineffassign\n"),
		OurContent: []byte("linters:\n  enable:\n    - errcheck\n    - govet\n    - mylinter\n"),
	})
	require.NoError(t, err)
	assert.Empty(t, out.Conflicts)
	s := string(out.Content)
	assert.True(t, strings.Contains(s, "ineffassign"), "missing ineffassign:\n%s", s)
	assert.True(t, strings.Contains(s, "mylinter"), "missing mylinter:\n%s", s)
}
