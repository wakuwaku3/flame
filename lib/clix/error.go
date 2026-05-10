package clix

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/wakuwaku3/flame/lib/ex"
)

// cliError は subcommand が flame プロセスの exit code を伝搬するための内部 error 表現 (FLM_FEA_0004)。 NewExitError 経由でのみ生成され、 ExitCodeOf / Main の errors.As target として使われる。 shellrun 経由で起動した shell の `*exec.ExitError` も shellrun.Run 内部で NewExitError に変換されるため、 clix package 内で扱う exit code 表現は本 type 1 種類に閉じる。
type cliError struct {
	err  error
	code int
}

func (e *cliError) Error() string { return e.err.Error() }
func (e *cliError) Unwrap() error { return e.err }

// NewExitError は subcommand が flame プロセスの exit code を伝搬するための error を返す。 leaf は usage 違反 / validation 失敗等の意図的な non-zero 終了経路で本関数を使い、 Main は当該 code を flame プロセス終了 code に複写する。
func NewExitError(code int) error {
	return &cliError{
		err:  ex.Errorf("exit code %d", code),
		code: code,
	}
}

// ExitCodeOf は err の chain 中に NewExitError 由来の cliError があれば (code, true) を、 無ければ (0, false) を返す。 service-level test (FLM_APP_0009) で exit code 副作用を検証する経路として公開する。 production の exit code 解決は Main が内包するため caller は本 helper を使う必要がない。
func ExitCodeOf(err error) (int, bool) {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code, true
	}
	return 0, false
}

// Main は flame CLI の main entrypoint helper。 execute (= root.Execute) を起動して戻り値の error を flame プロセス exit code に解決する。 exitCoder を満たす err は exit code のみを複写し stderr に追加出力しない (subcommand 側が既に違反内容を stderr に書いているため)。 それ以外の err は stderr に文字列出力後 exit 1。 os.Exit を直接呼ぶため main package のみから呼び出す前提 (test 経路は execute を直接呼んで error を検査する)。
func Main(ctx context.Context, execute func(ctx context.Context, cio IO) error) {
	cio := NewOSIO(ctx)
	err := execute(ctx, cio)
	if err == nil {
		return
	}
	if code, ok := ExitCodeOf(err); ok {
		os.Exit(code)
	}
	fmt.Fprintln(os.Stderr, err)
	const fallbackCode = 1
	os.Exit(fallbackCode)
}
