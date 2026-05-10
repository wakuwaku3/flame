package install

import (
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

func TestMerge3Way_YAML_Mapping_VendorAddsKey(t *testing.T) {
	t.Parallel()

	t.Run("vendor が新規 key 追加 + overlay は触れていない → vendor の key が install 結果に伝播する", func(t *testing.T) {
		t.Parallel()
		// Arrange
		input := &Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\n"),
			TheirContent: []byte("a: 1\nb: 2\n"),
			OurContent:   []byte("a: 1\n"),
		}
		// Act
		out, err := Merge3Way(input)
		// Assert
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Equal(t, "a: 1\nb: 2\n", string(out.Content))
	})

	t.Run("vendor が値変更 + overlay が key を削除 → conflict (vendor 値を出力)", func(t *testing.T) {
		t.Parallel()
		// Arrange
		input := &Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\n"),
			TheirContent: []byte("a: 2\n"),
			OurContent:   []byte("{}\n"),
		}
		// Act
		out, err := Merge3Way(input)
		// Assert
		require.NoError(t, err)
		require.Len(t, out.Conflicts, 1)
		assert.Equal(t, "a", out.Conflicts[0].Path)
		assert.Contains(t, out.Conflicts[0].Description, "their (vendor) changed")
	})
}

func TestMerge3Way_YAML_Mapping_OverlayRemovesKey(t *testing.T) {
	t.Parallel()

	t.Run("vendor 不変 + overlay が key 削除 → 利用者の削除を尊重 (key が消える)", func(t *testing.T) {
		t.Parallel()
		// Arrange
		input := &Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\nb: 2\n"),
			TheirContent: []byte("a: 1\nb: 2\n"),
			OurContent:   []byte("a: 1\n"),
		}
		// Act
		out, err := Merge3Way(input)
		// Assert
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.NotContains(t, string(out.Content), "b:")
	})

	t.Run("vendor が key 削除 + overlay は base と等価で kept → conflict なしで overlay 値を維持", func(t *testing.T) {
		t.Parallel()
		// Arrange
		input := &Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.yaml",
			BaseContent:  []byte("a: 1\nb: 2\n"),
			TheirContent: []byte("a: 1\n"),
			OurContent:   []byte("a: 1\nb: 2\n"),
		}
		// Act
		out, err := Merge3Way(input)
		// Assert: overlay = 「最終形」 semantics により、 利用者が overlay に b を残している以上は kept (conflict 出さず key 維持)
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Contains(t, string(out.Content), "b: 2")
	})
}

func TestMerge3Way_YAML_NestedMapping_NoBase(t *testing.T) {
	t.Parallel()

	// Arrange: 親 mapping は base にあるが nested key (`nested`) が base に無い + 子 value が mapping
	input := &Merge3WayInput{
		Strategy:     MergeDeep,
		InstallPath:  "x.yaml",
		BaseContent:  []byte("a: 1\n"),
		TheirContent: []byte("a: 1\nnested:\n  x: 10\n"),
		OurContent:   []byte("a: 1\nnested:\n  y: 20\n"),
	}
	// Act
	out, err := Merge3Way(input)
	// Assert: nil-safe 化された merge3WayYAMLMapping が baseMapNode=nil で正常に動作し、 vendor / overlay の両 key を merge する
	require.NoError(t, err)
	assert.Empty(t, out.Conflicts)
	s := string(out.Content)
	assert.Contains(t, s, "x: 10")
	assert.Contains(t, s, "y: 20")
}

func TestMerge3Way_YAML_NestedSequence_NoBase(t *testing.T) {
	t.Parallel()

	// Arrange: 親 mapping は base にあるが nested key (`items`) が base に無い + 子 value が sequence
	input := &Merge3WayInput{
		Strategy:     MergeDeep,
		InstallPath:  "x.yaml",
		BaseContent:  []byte("a: 1\n"),
		TheirContent: []byte("a: 1\nitems:\n  - x\n"),
		OurContent:   []byte("a: 1\nitems:\n  - y\n"),
	}
	// Act
	out, err := Merge3Way(input)
	// Assert: nil-safe 化された merge3WayYAMLSequence が baseSeqNode=nil で正常に動作し、 their / our 両要素を union する
	require.NoError(t, err)
	assert.Empty(t, out.Conflicts)
	s := string(out.Content)
	assert.Contains(t, s, "- x")
	assert.Contains(t, s, "- y")
}

func TestMerge3Way_YAML_NestedScalar_NoBase_Conflict(t *testing.T) {
	t.Parallel()

	// Arrange: ネストした key の base が存在しないが their / our 双方が異なる値で追加 → conflict
	input := &Merge3WayInput{
		Strategy:     MergeDeep,
		InstallPath:  "x.yaml",
		BaseContent:  []byte("root:\n  shared: 1\n"),
		TheirContent: []byte("root:\n  shared: 1\n  new: 2\n"),
		OurContent:   []byte("root:\n  shared: 1\n  new: 99\n"),
	}
	// Act
	out, err := Merge3Way(input)
	// Assert
	require.NoError(t, err)
	require.Len(t, out.Conflicts, 1)
	assert.Equal(t, "root.new", out.Conflicts[0].Path)
	assert.Contains(t, out.Conflicts[0].Description, "no base")
}

func TestMerge3Way_JSON_VendorAddsKey(t *testing.T) {
	t.Parallel()

	t.Run("vendor が新規 key 追加 + overlay は触れていない → vendor の key が install 結果に伝播する", func(t *testing.T) {
		t.Parallel()
		// Arrange
		input := &Merge3WayInput{
			Strategy:     MergeDeep,
			InstallPath:  "x.json",
			BaseContent:  []byte(`{"a":1}`),
			TheirContent: []byte(`{"a":1,"b":2}`),
			OurContent:   []byte(`{"a":1}`),
		}
		// Act
		out, err := Merge3Way(input)
		// Assert
		require.NoError(t, err)
		assert.Empty(t, out.Conflicts)
		assert.Contains(t, string(out.Content), `"b"`)
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

	// Arrange
	input := &Merge3WayInput{
		Strategy:     MergeDeep,
		InstallPath:  "x.yaml",
		BaseContent:  []byte("linters:\n  enable:\n    - errcheck\n    - govet\n"),
		TheirContent: []byte("linters:\n  enable:\n    - errcheck\n    - govet\n    - ineffassign\n"),
		OurContent:   []byte("linters:\n  enable:\n    - errcheck\n    - govet\n    - mylinter\n"),
	}
	// Act
	out, err := Merge3Way(input)
	// Assert
	require.NoError(t, err)
	assert.Empty(t, out.Conflicts)
	s := string(out.Content)
	assert.Contains(t, s, "ineffassign", "missing ineffassign in:\n%s", s)
	assert.Contains(t, s, "mylinter", "missing mylinter in:\n%s", s)
}

func TestMerge3Way_UnknownStrategy(t *testing.T) {
	t.Parallel()

	// Arrange
	input := &Merge3WayInput{
		Strategy:     MergeStrategy("invalid"),
		InstallPath:  "x.yaml",
		BaseContent:  nil,
		TheirContent: []byte("a: 1\n"),
		OurContent:   nil,
	}
	// Act
	_, err := Merge3Way(input)
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown merge strategy")
}

func TestMerge3Way_JSON_VendorRemoved_OurKept_Conflict(t *testing.T) {
	t.Parallel()

	// Arrange
	input := &Merge3WayInput{
		Strategy:     MergeDeep,
		InstallPath:  "x.json",
		BaseContent:  []byte(`{"items":["a","b"]}`),
		TheirContent: []byte(`{"items":["a"]}`),
		OurContent:   []byte(`{"items":["a","b"]}`),
	}
	// Act
	out, err := Merge3Way(input)
	// Assert
	require.NoError(t, err)
	require.NotEmpty(t, out.Conflicts)
	assert.Contains(t, out.Conflicts[0].Description, "their")
}

func TestMerge3Way_JSON_ScalarBothChanged_Conflict(t *testing.T) {
	t.Parallel()

	// Arrange
	input := &Merge3WayInput{
		Strategy:     MergeDeep,
		InstallPath:  "x.json",
		BaseContent:  []byte(`{"a":1}`),
		TheirContent: []byte(`{"a":2}`),
		OurContent:   []byte(`{"a":99}`),
	}
	// Act
	out, err := Merge3Way(input)
	// Assert
	require.NoError(t, err)
	require.Len(t, out.Conflicts, 1)
	assert.Equal(t, "a", out.Conflicts[0].Path)
}
