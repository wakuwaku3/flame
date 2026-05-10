// Package ex は stacktrace 付き error を扱う wrapper。 FLM_APP_0007 §error
// 表現と stacktrace に従い、 module 横断で本 package のみを介して error を生成
// / wrap する。
//
// 本 package は 3rd party に依存せず、 標準ライブラリ (errors / fmt / runtime)
// のみで実装する。 内部表現 (stackError 型) は package private で、 caller は
// New / Errorf / Wrap / Wrapf 関数経由でのみ生成し、 errors.Is / errors.As /
// fmt.Sprintf 系で値を扱う。
package ex

import (
	"errors"
	"fmt"
	"io"
	"runtime"
)

const (
	stackTraceDepth = 64
	// captureStackSkip は runtime.Callers に渡す skip 値。 runtime.Callers 自身、
	// captureStack、 公開関数本体 (New / Errorf / Wrap / Wrapf) の 3 frame を飛ばし、
	// caller の呼び出し位置を最上 frame にする。
	captureStackSkip = 3
)

type stackError struct {
	wrapped error
	msg     string
	stack   []uintptr
}

var _ fmt.Formatter = (*stackError)(nil)

func (e *stackError) Error() string {
	return e.msg
}

func (e *stackError) Unwrap() error {
	return e.wrapped
}

// Format の verb 別出力:
//   - `%s` / `%v`: msg のみ
//   - `%+v`: msg + stacktrace を改行区切りで出力
//   - `%q`: msg を quote
//
// fmt.State への書き込み error は in-memory buffer 相当で慣習的に握り潰す
// (FLM_GEN_0006 §lint の局所抑制 を回避するため、 errcheck の exclude
// 設定で除外している)。
func (e *stackError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = io.WriteString(s, e.msg)
			writeStack(s, e.stack)
			return
		}
		fallthrough
	case 's':
		_, _ = io.WriteString(s, e.msg)
	case 'q':
		fmt.Fprintf(s, "%q", e.msg)
	}
}

func New(msg string) error {
	return &stackError{
		msg:     msg,
		wrapped: nil,
		stack:   captureStack(),
	}
}

// Errorf は `%w` verb で wrap した場合 (multi-%w / `errors.Join` 経由 含む)、
// fmt.Errorf の戻り値を unwrap chain に保持する。 errors.Is / errors.As は
// 本値 → fmt.Errorf 戻り値 → 元 error と辿り、 multi-%w (Go 1.20+) も
// `Unwrap() []error` interface 経由で正しく解決される。
func Errorf(format string, args ...any) error {
	inner := fmt.Errorf(format, args...)
	return &stackError{
		msg:     inner.Error(),
		wrapped: inner,
		stack:   captureStack(),
	}
}

// Wrap は既に chain 中に stacktrace を持つ場合、 再 wrap せず元の err をそのまま
// 返す (二重 stack 付与を避けるため)。
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	if hasStack(err) {
		return err
	}
	return &stackError{
		msg:     err.Error(),
		wrapped: err,
		stack:   captureStack(),
	}
}

// Wrapf は Wrap と異なり message 付与が目的のため、 chain 中に stack があっても
// 呼び出し点の stack を新たに捕捉する。
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &stackError{
		msg:     fmt.Sprintf(format, args...) + ": " + err.Error(),
		wrapped: err,
		stack:   captureStack(),
	}
}

func captureStack() []uintptr {
	pcs := make([]uintptr, stackTraceDepth)
	n := runtime.Callers(captureStackSkip, pcs)
	return pcs[:n]
}

func hasStack(err error) bool {
	var se *stackError
	return errors.As(err, &se)
}

func writeStack(w io.Writer, pcs []uintptr) {
	if len(pcs) == 0 {
		return
	}
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		fmt.Fprintf(w, "\n%s\n\t%s:%d", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
}
