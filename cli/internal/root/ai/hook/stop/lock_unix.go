//go:build !windows

package stop

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/ex"
)

// acquireLock は flock(LOCK_EX|LOCK_NB) を lockPollEvery 周期で lockTimeout までリトライする。 ctx cancel は poll 待機を即時打ち切って取得失敗 (= 非ブロック扱い) で返す。 syscall.Flock は POSIX 専用なので Windows ビルドでは別実装 (stub) に分岐する (flame は Linux / macOS の Claude Code hook で利用される前提のため、 Windows でのファイルロック挙動は no-op で許容)。
func acquireLock(ctx context.Context, path string) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, fsperm.File) //nolint:gosec // G304: stop hook が排他のために生成する固定 path (state_dir/.lock) で、 caller 制御下の任意値ではなく内部で組み立てた固定 path のため。
	if err != nil {
		return nil, false, ex.Wrap(err)
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) //nolint:gosec // G115: syscall.Flock の API constraint (file descriptor は int 引数)。 file.Fd() の uintptr → int 変換は実 fd が int 範囲 (linux 上限 1024 程度) のため overflow しない。
		if flockErr == nil {
			return f, true, nil
		}
		if !errors.Is(flockErr, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, false, ex.Wrap(flockErr)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, false, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, false, nil
		case <-time.After(lockPollEvery):
		}
	}
}

func releaseLock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:gosec // G115: 同上、 syscall.Flock の API constraint。
	_ = f.Close()
}
