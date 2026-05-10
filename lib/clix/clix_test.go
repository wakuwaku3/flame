package clix_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("--version で root の use と version が出力される", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.2.3"))
		fake := clix.NewFakeIO(t, []string{"--version"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "demo 1.2.3\n", "")
	})

	t.Run("__spec で flame CLI の公開 surface JSON が出力され、 内部 endpoint が除外される", func(t *testing.T) {
		t.Parallel()

		// Arrange
		// cobra IsAvailableCommand が RunE 未設定の subcommand を spec / help から
		// 除外するため、 alpha を surface に乗せるには no-op RunE を設定する。
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.2.3"))
		r.AddCommand(clix.NewCommand(clix.NewCommandConfig(
			"alpha",
			clix.WithCommandRunE(func(_ context.Context, _ clix.RunInput) error { return nil }),
		)))
		fake := clix.NewFakeIO(t, []string{"__spec"})
		const expectedSpecJSON = `{
  "name": "demo",
  "path": "demo",
  "subcommands": [
    {
      "name": "alpha",
      "path": "demo alpha",
      "subcommands": [],
      "flags": []
    },
    {
      "name": "completion",
      "path": "demo completion",
      "subcommands": [
        {
          "name": "bash",
          "path": "demo completion bash",
          "subcommands": [],
          "flags": [
            {
              "name": "no-descriptions",
              "type": "bool",
              "required": false
            }
          ]
        },
        {
          "name": "fish",
          "path": "demo completion fish",
          "subcommands": [],
          "flags": [
            {
              "name": "no-descriptions",
              "type": "bool",
              "required": false
            }
          ]
        },
        {
          "name": "powershell",
          "path": "demo completion powershell",
          "subcommands": [],
          "flags": [
            {
              "name": "no-descriptions",
              "type": "bool",
              "required": false
            }
          ]
        },
        {
          "name": "zsh",
          "path": "demo completion zsh",
          "subcommands": [],
          "flags": [
            {
              "name": "no-descriptions",
              "type": "bool",
              "required": false
            }
          ]
        }
      ],
      "flags": []
    }
  ],
  "flags": []
}
`

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, expectedSpecJSON, "")
	})

	t.Run("subcommand RunE が positional args を受け取れる", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var captured []string
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.0.0"))
		r.AddCommand(clix.NewCommand(clix.NewCommandConfig(
			"echo",
			clix.WithCommandRunE(func(_ context.Context, in clix.RunInput) error {
				captured = append([]string{}, in.Args()...)
				return nil
			}),
		)))
		fake := clix.NewFakeIO(t, []string{"echo", "alpha", "beta"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []string{"alpha", "beta"}, captured)
	})

	t.Run("subcommand RunE の error が propagate される", func(t *testing.T) {
		t.Parallel()

		// Arrange
		sentinel := errors.New("subcommand boom")
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.0.0"))
		r.AddCommand(clix.NewCommand(clix.NewCommandConfig(
			"fail",
			clix.WithCommandRunE(func(_ context.Context, _ clix.RunInput) error { return sentinel }),
		)))
		fake := clix.NewFakeIO(t, []string{"fail"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("未定義の subcommand で error が返る", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.0.0"))
		fake := clix.NewFakeIO(t, []string{"no-such-command"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.Error(t, err)
	})

	t.Run("RunE が FakeIO.SetStdin で注入された stdin を読み取れる", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var captured []byte
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.0.0"))
		r.AddCommand(clix.NewLeaf(
			"read-stdin",
			"read all stdin",
			func(_ context.Context, in clix.RunInput) error {
				b, err := io.ReadAll(in.Stdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				captured = b
				return nil
			},
		))
		fake := clix.NewFakeIO(t, []string{"read-stdin"})
		fake.SetStdin(t, strings.NewReader("hello stdin\n"))

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "hello stdin\n", string(captured))
		fake.Verify(t, "", "")
	})

	t.Run("FakeIO の default stdin は空 reader として振る舞う", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var captured []byte
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.0.0"))
		r.AddCommand(clix.NewLeaf(
			"read-stdin-empty",
			"read all stdin",
			func(_ context.Context, in clix.RunInput) error {
				b, err := io.ReadAll(in.Stdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				captured = b
				return nil
			},
		))
		fake := clix.NewFakeIO(t, []string{"read-stdin-empty"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, captured)
		fake.Verify(t, "", "")
	})

	t.Run("FakeIO.SetStdin に nil reader を渡すと Fatalf で test を即座に失敗させる", func(t *testing.T) {
		t.Parallel()

		// Arrange
		// testing.TB は sealed interface のため、 真の testing.T (の TB 投影) を embed して
		// Helper / Fatalf のみ override し、 Fatalf 呼び出しを捕捉する。
		fake := clix.NewFakeIO(t, []string{})
		stub := &fakeTBRecorder{TB: t, fatalfCalled: false}

		// Act
		fake.SetStdin(stub, nil)

		// Assert
		assert.True(t, stub.fatalfCalled)
	})

	t.Run("FakeIO.StdoutString / StderrString が buffer 内容を返す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.2.3"))
		fake := clix.NewFakeIO(t, []string{"--version"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "demo 1.2.3\n", fake.StdoutString(t))
		assert.Empty(t, fake.StderrString(t))
	})

	t.Run("NewLeaf 経由で登録された subcommand が args を受け取り error を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var captured []string
		sentinel := errors.New("leaf boom")
		r := clix.NewRoot(clix.NewRootConfig("demo", "1.0.0"))
		r.AddCommand(clix.NewLeaf(
			"echo-fail",
			"echo args then fail",
			func(_ context.Context, in clix.RunInput) error {
				captured = append([]string{}, in.Args()...)
				return sentinel
			},
		))
		fake := clix.NewFakeIO(t, []string{"echo-fail", "alpha", "beta"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, []string{"alpha", "beta"}, captured)
	})
}

// fakeTBRecorder は testing.TB の Fatalf 呼び出しを捕捉するための test 用 stub。
// testing.TB は private() を持つ sealed interface のため、 真の testing.TB を embed する形で
// Helper / Fatalf のみ override する。 method 宣言は Go の言語制約上 package scope に置く必要が
// あるため、 単一テストケース利用 (FLM_APP_0009 §test 内 identifier scope) の例外として top-level
// に置く。
type fakeTBRecorder struct {
	testing.TB
	fatalfCalled bool
}

func (f *fakeTBRecorder) Helper()                   {}
func (f *fakeTBRecorder) Fatalf(_ string, _ ...any) { f.fatalfCalled = true }
