//go:build windows

package stop

import (
	"context"
	"os"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/ex"
)

// acquireLock の Windows 向け stub。 flame の Stop hook は Linux / macOS の Claude Code 実行環境を前提に実装されており、 Windows native では Claude Code 自体の対応がほぼ無いため (WSL 経由の利用が標準)、 排他制御は no-op で良い。 file は opened state で返し、 caller の defer releaseLock(f) で close されるよう挙動を揃える (= シリアル実行時の通常 path は同じ動作になる)。
func acquireLock(_ context.Context, path string) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, fsperm.File) //nolint:gosec // G304: stop hook が排他のために生成する固定 path (state_dir/.lock) で、 caller 制御下の任意値ではなく内部で組み立てた固定 path のため。
	if err != nil {
		return nil, false, ex.Wrap(err)
	}
	return f, true, nil
}

func releaseLock(f *os.File) {
	_ = f.Close()
}
