package lib_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/ci/release/spec/lib"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("module-path 引数で指定した library module の Go 公開 API surface JSON が emit される", func(t *testing.T) {
		t.Parallel()

		// Arrange
		// fixture は test 実行時に t.TempDir() 配下へ動的生成する library
		// module。 静的 testdata/ に Go module を置くと当該 module が CI 検査
		// (golangci-lint / detect.sh 等) の走査対象に巻き込まれて干渉する。
		const goMod = "module example.com/fixture\n\ngo 1.26\n"
		const rootGo = `package fixture

import "io"

type Foo struct {
	Bar     string
	private int
}

type Reader interface {
	io.Reader
	Greet() string
}

func Hello() string {
	return "hello"
}

func (f *Foo) Greet() string {
	return f.Bar
}
`
		const subGo = `package sub

func Sub() string {
	return "sub"
}
`
		const internalGo = `package hidden

func Inside() string {
	return "hidden"
}
`
		fixturePath := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(fixturePath, "go.mod"), []byte(goMod), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(fixturePath, "root.go"), []byte(rootGo), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(fixturePath, "sub"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(fixturePath, "sub", "sub.go"), []byte(subGo), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(fixturePath, "internal"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(fixturePath, "internal", "hidden.go"), []byte(internalGo), 0o600))

		r := clix.NewRoot(clix.NewRootConfig("flame", "0.0.0-test"))
		r.AddCommand(lib.New())
		fake := clix.NewFakeIO(t, []string{"lib", fixturePath})

		const expectedStdout = `{
  "module": "example.com/fixture",
  "packages": [
    {
      "path": "",
      "identifiers": [
        {
          "name": "Foo.Bar",
          "kind": "field",
          "signature": "string"
        },
        {
          "name": "Hello",
          "kind": "func",
          "signature": "func() string"
        },
        {
          "name": "Reader.Reader",
          "kind": "interface_embed",
          "signature": "io.Reader"
        },
        {
          "name": "Reader.Greet",
          "kind": "interface_method",
          "signature": "func() string"
        },
        {
          "name": "(*Foo).Greet",
          "kind": "method",
          "signature": "func() string"
        },
        {
          "name": "Foo",
          "kind": "type",
          "signature": "struct"
        },
        {
          "name": "Reader",
          "kind": "type",
          "signature": "interface"
        }
      ]
    },
    {
      "path": "sub",
      "identifiers": [
        {
          "name": "Sub",
          "kind": "func",
          "signature": "func() string"
        }
      ]
    }
  ]
}
`

		// Act
		runErr := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, runErr)
		fake.Verify(t, expectedStdout, "")
	})

	t.Run("引数なしで error", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "0.0.0-test"))
		r.AddCommand(lib.New())
		fake := clix.NewFakeIO(t, []string{"lib"})

		// Act
		runErr := r.Run(t.Context(), fake)

		// Assert
		require.Error(t, runErr)
		fake.Verify(t, "", "")
	})

	t.Run("module-path に存在しない directory を渡すと error", func(t *testing.T) {
		t.Parallel()

		// Arrange
		nonexistent := filepath.Join(t.TempDir(), "absent")

		r := clix.NewRoot(clix.NewRootConfig("flame", "0.0.0-test"))
		r.AddCommand(lib.New())
		fake := clix.NewFakeIO(t, []string{"lib", nonexistent})

		// Act
		runErr := r.Run(t.Context(), fake)

		// Assert
		require.Error(t, runErr)
		fake.Verify(t, "", "")
	})

	t.Run("go.mod に module directive が無いと error", func(t *testing.T) {
		t.Parallel()

		// Arrange
		fixturePath := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(fixturePath, "go.mod"), []byte("go 1.26\n"), 0o600))

		r := clix.NewRoot(clix.NewRootConfig("flame", "0.0.0-test"))
		r.AddCommand(lib.New())
		fake := clix.NewFakeIO(t, []string{"lib", fixturePath})

		// Act
		runErr := r.Run(t.Context(), fake)

		// Assert
		require.Error(t, runErr)
		fake.Verify(t, "", "")
	})
}
