package ex_test

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wakuwaku3/flame/lib/ex"
)

func TestNewAttachesStack(t *testing.T) {
	t.Parallel()

	// Arrange
	const msg = "boom"

	// Act
	err := ex.New(msg)
	got := fmt.Sprintf("%+v", err)

	// Assert
	assert.Contains(t, got, msg)
	assert.Contains(t, got, "TestNewAttachesStack")
}

func TestErrorfPreservesUnwrapChain(t *testing.T) {
	t.Parallel()

	// Arrange
	const expectedMsg = "read failed: EOF"
	base := io.EOF

	// Act
	err := ex.Errorf("read failed: %w", base)

	// Assert
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, expectedMsg, err.Error())
}

func TestErrorfPreservesMultipleWrapTargets(t *testing.T) {
	t.Parallel()

	// Arrange
	a := errors.New("a")
	b := errors.New("b")

	// Act
	err := ex.Errorf("combined: %w; %w", a, b)

	// Assert
	require.ErrorIs(t, err, a)
	require.ErrorIs(t, err, b)
}

func TestWrapNilReturnsNil(t *testing.T) {
	t.Parallel()

	// Arrange
	// (no setup; nil 入力に対する挙動を確認する)

	// Act
	gotWrap := ex.Wrap(nil)
	gotWrapf := ex.Wrapf(nil, "x")

	// Assert
	assert.NoError(t, gotWrap, "ex.Wrap(nil) should return nil")
	assert.NoError(t, gotWrapf, "ex.Wrapf(nil, ...) should return nil")
}

func TestWrapDoesNotDoubleAttachStack(t *testing.T) {
	t.Parallel()

	// Arrange
	first := ex.New("first")

	// Act
	second := ex.Wrap(first)

	// Assert
	// errorlint が `!=` での error 比較を禁止するため any にボクシングして runtime identity を比較する。
	assert.Equal(t, any(first), any(second), "expected Wrap to return same error when chain already has stack")
}

func TestWrapfAddsMessageAndChain(t *testing.T) {
	t.Parallel()

	// Arrange
	const expectedMsg = "context: 42: base"
	base := errors.New("base")

	// Act
	err := ex.Wrapf(base, "context: %d", 42)

	// Assert
	require.ErrorIs(t, err, base)
	assert.Equal(t, expectedMsg, err.Error())
}

func TestFormatPlainV(t *testing.T) {
	t.Parallel()

	// Arrange
	const msg = "hello"
	err := ex.New(msg)

	// Act
	gotV := fmt.Sprintf("%v", err)
	gotS := fmt.Sprintf("%s", err)

	// Assert
	assert.Equal(t, msg, gotV)
	assert.Equal(t, msg, gotS)
}

func TestFormatQuotesMessage(t *testing.T) {
	t.Parallel()

	// Arrange
	// 改行・引用符を含む message を quote 対象として与え、 fmt の %q と同じ
	// quote 結果が得られることを確認する。
	const msg = "he said \"hi\"\nbye"
	err := ex.New(msg)
	expected := fmt.Sprintf("%q", msg)

	// Act
	got := fmt.Sprintf("%q", err)

	// Assert
	assert.Equal(t, expected, got)
}

func TestFormatPlusVIncludesStackFrame(t *testing.T) {
	t.Parallel()

	// Arrange
	const msg = "framed"
	err := ex.New(msg)

	// Act
	got := fmt.Sprintf("%+v", err)

	// Assert
	// `%+v` は msg に続けて少なくとも 1 frame を `\n<func>\n\t<file>:<line>`
	// 形式で出力する。 frame の関数名 / ファイル名 / 行番号が出力に含まれ、
	// かつ frame 1 件あたりの形式が production 仕様 (writeStack) と一致することを
	// 1 つの output 全体に対する正規表現一致で検証する。
	require.True(t, strings.HasPrefix(got, msg), "output must start with msg, got %q", got)

	// 先頭の msg 直後から少なくとも 1 frame の `\n<func>\n\t<file>:<line>` を要求する。
	framePattern := regexp.MustCompile(`(?m)\A` + regexp.QuoteMeta(msg) + `(?:\n[^\n\t]+\n\t[^\n]+:\d+)+\z`)
	assert.Regexp(t, framePattern, got)

	// 最上位 frame には本テスト関数名・本ファイル名が含まれる。
	assert.Contains(t, got, "TestFormatPlusVIncludesStackFrame")
	assert.Contains(t, got, "ex_test.go")
}
