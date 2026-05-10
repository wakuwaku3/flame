package dispatch_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/check/bucketize"
	"github.com/wakuwaku3/flame/cli/internal/check/dispatch"
)

// service-level test (FLM_APP_0009): dispatch 層は (1) 空入力 / 未知 key の早期 fail、 (2) 既知 checker への in-process invocation で stdout / stderr / exit code を集約する hot path、 (3) 並列実行で複数 entry の exit code を max で集約、 を覆う。 個別 checker の挙動は各 cli/internal/root/check/* の test に閉じるため、 本 test は外部 tool 依存の無い json checker (= encoding/json で完結) を代表として in-process 経路を verify する。
//
//nolint:paralleltest // 共有 registry を介する dispatch は process global 副作用を持つため parallel 不可
func TestDispatch(t *testing.T) {
	t.Run("空 entries は output 空 / code 0 / err nil で素通り", func(t *testing.T) {
		// Act
		out, code, err := dispatch.Dispatch(context.Background(), nil, 1)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, out)
		assert.Equal(t, 0, code)
	})

	t.Run("registry に未登録の Checker 名は error を返す", func(t *testing.T) {
		// Arrange
		entries := []bucketize.Entry{{Checker: "nonexistent.sh", Targets: []string{"foo"}}}

		// Act
		_, _, err := dispatch.Dispatch(context.Background(), entries, 1)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent.sh")
	})

	t.Run("正常 invocation は in-process で exit 0 を返す", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		validPath := filepath.Join(dir, "valid.json")
		require.NoError(t, os.WriteFile(validPath, []byte(`{"ok":true}`), 0o600))
		entries := []bucketize.Entry{{Checker: bucketize.CheckerJSON, Targets: []string{validPath}}}

		// Act
		out, code, err := dispatch.Dispatch(context.Background(), entries, 1)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, out)
		assert.Equal(t, 0, code)
	})

	t.Run("checker が NewExitError を返した場合は同 code が伝搬する", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		invalidPath := filepath.Join(dir, "invalid.json")
		require.NoError(t, os.WriteFile(invalidPath, []byte(`{not-json`), 0o600))
		entries := []bucketize.Entry{{Checker: bucketize.CheckerJSON, Targets: []string{invalidPath}}}

		// Act
		out, code, err := dispatch.Dispatch(context.Background(), entries, 1)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 1, code)
		assert.Contains(t, out, "FAIL: ")
		assert.Contains(t, out, "invalid JSON")
	})

	t.Run("複数 entry の exit code は max で集約される", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		validPath := filepath.Join(dir, "valid.json")
		invalidPath := filepath.Join(dir, "invalid.json")
		require.NoError(t, os.WriteFile(validPath, []byte(`{"ok":true}`), 0o600))
		require.NoError(t, os.WriteFile(invalidPath, []byte(`{not-json`), 0o600))
		// 同 checker を 2 entry に分けることで dispatcher 内部の goroutine 並列実行と code 集約経路が走る (1 entry が exit 0、 もう 1 つが exit 1 → max = 1)。
		entries := []bucketize.Entry{
			{Checker: bucketize.CheckerJSON, Targets: []string{validPath}},
			{Checker: bucketize.CheckerJSON, Targets: []string{invalidPath}},
		}

		// Act
		out, code, err := dispatch.Dispatch(context.Background(), entries, 2)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 1, code)
		assert.Contains(t, out, "invalid JSON")
	})
}
