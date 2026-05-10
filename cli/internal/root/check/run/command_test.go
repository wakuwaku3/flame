package run_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/check/bucketize"
	checkroot "github.com/wakuwaku3/flame/cli/internal/root/check"
	"github.com/wakuwaku3/flame/lib/clix"
)

// service-level test (FLM_APP_0009): `flame check run <key>` の入力空間を主要分岐で覆う (usage 不一致 / 未知 key / FILES_JSON 不在 / 不正 JSON / 空 list / 正常 dispatch / leading dash 拒否 / checker exit code 伝搬)。 dispatch 経路の検証は dispatch package の test に閉じるため、 本 test は外部 tool 依存の無い json checker (= encoding/json で完結) を代表に in-process invocation を verify する。
//
//nolint:paralleltest // FILES_JSON env が process global 副作用を持つため parallel 不可
func TestRun(t *testing.T) {
	t.Run("引数が無いと usage を出して exit 2", func(t *testing.T) {
		// Arrange
		t.Setenv("FILES_JSON", "[]")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", "usage: flame check run <bucket-key> (FILES_JSON env required)\n")
	})

	t.Run("未知の bucket-key は usage 失敗 (exit 2) を返す", func(t *testing.T) {
		// Arrange
		t.Setenv("FILES_JSON", "[]")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run", "nonexistent"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", "error: unknown checker key \"nonexistent\"\n")
	})

	t.Run("FILES_JSON env 不在は failure (exit 1) を返す", func(t *testing.T) {
		// Arrange
		t.Setenv("FILES_JSON", "")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run", bucketize.CheckerJSON})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), "FILES_JSON env must be set")
	})

	t.Run("FILES_JSON が JSON 配列でないと failure (exit 1) を返す", func(t *testing.T) {
		// Arrange
		t.Setenv("FILES_JSON", `{"not":"array"}`)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run", bucketize.CheckerJSON})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), "FILES_JSON is not a JSON string array")
	})

	t.Run("空 list の FILES_JSON は no-op success (exit 0) を返す", func(t *testing.T) {
		// Arrange
		t.Setenv("FILES_JSON", "[]")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run", bucketize.CheckerJSON})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		assert.Contains(t, fake.StderrString(t), "no files to check for "+bucketize.CheckerJSON)
	})

	t.Run("先頭が '-' のファイル名は CLI flag と衝突しうるため failure (exit 1) を返す", func(t *testing.T) {
		// Arrange
		t.Setenv("FILES_JSON", `["-rf"]`)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run", bucketize.CheckerJSON})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), "invalid file name starting with '-'")
	})

	t.Run("正常 invocation は対応 checker に in-process dispatch される (json checker を代表に)", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		validPath := filepath.Join(dir, "valid.json")
		require.NoError(t, os.WriteFile(validPath, []byte(`{"ok":true}`), 0o600))
		t.Setenv("FILES_JSON", `["`+validPath+`"]`)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run", bucketize.CheckerJSON})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("checker 由来の exit code は伝搬する (json checker の不正 JSON で exit 1)", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		invalidPath := filepath.Join(dir, "invalid.json")
		require.NoError(t, os.WriteFile(invalidPath, []byte(`{not-json`), 0o600))
		t.Setenv("FILES_JSON", `["`+invalidPath+`"]`)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(checkroot.New())
		fake := clix.NewFakeIO(t, []string{"check", "run", bucketize.CheckerJSON})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), "FAIL: ")
		assert.Contains(t, fake.StderrString(t), "invalid JSON")
	})
}
