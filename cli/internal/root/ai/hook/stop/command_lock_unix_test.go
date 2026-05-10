//go:build !windows

package stop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 別 process / 並行 stop hook が flock を保持中という現実シナリオを fake repo 上で再現するための test。 acquireLock 内 lockTimeout (60s) を test 時間に持ち込まないよう、 ctx を即 cancel して `<-ctx.Done()` 経路で lockOK=false を返させる (lock_unix.go §poll 待機の cancel 経路)。 排他制御の前提が崩れる Windows ビルド (lock_windows.go で no-op) では本 test は意味を成さないため build constraint で除外。
//
//nolint:paralleltest // cwd / env 変更を伴うため parallel 不可
func TestDoRun_LockAcquireFailure(t *testing.T) {
	// Arrange
	repo := setupRepo(t)
	writeWorking(t, repo, "lib/x.txt", "hello\n")
	seedStateForTrackedFiles(t, repo)
	writeWorking(t, repo, "lib/x.txt", "changed\n")
	t.Setenv("CLAUDE_PROJECT_DIR", repo)
	holdStopHookLock(t, repo)
	fake, check := newFakeAndCheck(t)
	fake.SetStdin(t, strings.NewReader(`{}`))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Act
	err := runWithFakeCheck(ctx, fake, check.fn)

	// Assert
	require.NoError(t, err)
	fake.Verify(t, "", "stop-hook: failed to acquire lock within 60s\n")
	assert.Equal(t, int32(0), check.callCount.Load(), "lock 取得失敗時 checkFn は呼ばれないこと")
}

// holdStopHookLock は stop hook が触る state_dir/.lock を test goroutine 側で先取得し、 production の acquireLock を EWOULDBLOCK 経路に追い込む fixture。 t.Cleanup で release を必ず予約することで、 後続 test (= 連続実行時) の lock 残留を防ぐ。
func holdStopHookLock(tb testing.TB, repo string) {
	tb.Helper()
	stateDir := filepath.Join(repo, ".claude", ".cache", "stop-hook")
	require.NoError(tb, os.MkdirAll(stateDir, 0o750))
	f, err := os.OpenFile(filepath.Join(stateDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(tb, err)
	require.NoError(tb, syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)) //nolint:gosec // G115: 同 package lock_unix.go と同じく syscall.Flock の API constraint。 file.Fd() の uintptr → int 変換は実 fd が int 範囲のため overflow しない。
	tb.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:gosec // G115: 同上。
		_ = f.Close()
	})
}
