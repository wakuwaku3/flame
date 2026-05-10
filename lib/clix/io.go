package clix

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// IO は public method を持たない sealed interface
// (FLM_APP_0007 §premature publishing)。
// 実装は osIO (本番) と FakeIO (test fake) に限定される。
type IO interface {
	args() []string
	stdin() io.Reader
	stdout() io.Writer
	stderr() io.Writer
}

type osIO struct {
	stdinR  io.Reader
	stdoutW io.Writer
	stderrW io.Writer
	argv    []string
}

var _ IO = (*osIO)(nil)

// NewOSIO の ctx は公開 entry point の signature を ctx 第一引数で揃えるため
// (FLM_APP_0007 §context 伝搬)。
func NewOSIO(_ context.Context) *osIO {
	return &osIO{argv: os.Args[1:], stdinR: os.Stdin, stdoutW: os.Stdout, stderrW: os.Stderr}
}

func (o *osIO) args() []string    { return o.argv }
func (o *osIO) stdin() io.Reader  { return o.stdinR }
func (o *osIO) stdout() io.Writer { return o.stdoutW }
func (o *osIO) stderr() io.Writer { return o.stderrW }

// FakeIO は IO の in-memory fake
// (FLM_APP_0009 §mock を採用しない / fake を採用する)。
type FakeIO struct {
	stdinR  io.Reader
	stdoutW *bytes.Buffer
	stderrW *bytes.Buffer
	argv    []string
}

var _ IO = (*FakeIO)(nil)

// NewFakeIO は第一引数 tb で production code 経路から呼べないことを
// compile-time に保証する (FLM_APP_0009 §test helper signature)。
func NewFakeIO(tb testing.TB, args []string) *FakeIO {
	tb.Helper()
	return &FakeIO{
		argv:    args,
		stdinR:  bytes.NewReader(nil),
		stdoutW: &bytes.Buffer{},
		stderrW: &bytes.Buffer{},
	}
}

// SetStdin は stdin から読み取る subcommand の test で fake stdin を注入する経路 (FLM_APP_0009 §service-level test の writer 注入経路)。 nil reader は呼び出し側のミスとして検出するため、 NewFakeIO の default (= 空 reader) との上書きを明示する API にする。
func (f *FakeIO) SetStdin(tb testing.TB, r io.Reader) {
	tb.Helper()
	if r == nil {
		tb.Fatalf("SetStdin: reader must not be nil")
	}
	f.stdinR = r
}

func (f *FakeIO) args() []string    { return f.argv }
func (f *FakeIO) stdin() io.Reader  { return f.stdinR }
func (f *FakeIO) stdout() io.Writer { return f.stdoutW }
func (f *FakeIO) stderr() io.Writer { return f.stderrW }

// Verify は stdout / stderr を 1 メソッドで合算検証する
// (FLM_APP_0009 §assertion 規約)。
func (f *FakeIO) Verify(tb testing.TB, expectedStdout, expectedStderr string) {
	tb.Helper()
	assert.Equal(tb, expectedStdout, f.stdoutW.String(), "stdout mismatch")
	assert.Equal(tb, expectedStderr, f.stderrW.String(), "stderr mismatch")
}

// StdoutString は parity 比較等で expected を test ケース内で動的に組み立てる必要がある経路 (例: shell 版の出力と Go 版の出力を直接比較する) のための inspection method。 通常の service-level test は Verify を使い、 本 method は parity test 専用の補助とする (FLM_FEA_0004 §影響: 配線替え時に shell 版と Go 版の挙動同一性を確認する経路)。
func (f *FakeIO) StdoutString(tb testing.TB) string {
	tb.Helper()
	return f.stdoutW.String()
}

func (f *FakeIO) StderrString(tb testing.TB) string {
	tb.Helper()
	return f.stderrW.String()
}
