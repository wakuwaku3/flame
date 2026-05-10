package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinUsesRef(t *testing.T) {
	t.Parallel()

	const sourceRepo = "github.com/wakuwaku3/flame"

	t.Run("self version は @main で書き換える (dogfooding)", func(t *testing.T) {
		t.Parallel()
		input := []byte("uses: wakuwaku3/flame/.github/workflows/wf__check.yaml@v1.2.0\n")
		out, err := pinUsesRef(input, sourceRepo, SelfVersion)
		require.NoError(t, err)
		assert.Equal(t, "uses: wakuwaku3/flame/.github/workflows/wf__check.yaml@main\n", string(out))
	})

	t.Run("downstream version は @<version> で書き換える", func(t *testing.T) {
		t.Parallel()
		input := []byte("uses: wakuwaku3/flame/.github/workflows/wf__check.yaml@main\n")
		out, err := pinUsesRef(input, sourceRepo, "v3.1.4")
		require.NoError(t, err)
		assert.Equal(t, "uses: wakuwaku3/flame/.github/workflows/wf__check.yaml@v3.1.4\n", string(out))
	})

	t.Run("複数の uses 行を一括で書き換える", func(t *testing.T) {
		t.Parallel()
		input := []byte(`jobs:
  a:
    uses: wakuwaku3/flame/.github/workflows/wf__a.yaml@main
  b:
    uses: wakuwaku3/flame/.github/workflows/wf__b.yaml@v0.1.0
`)
		out, err := pinUsesRef(input, sourceRepo, "v9.9.9")
		require.NoError(t, err)
		s := string(out)
		assert.Contains(t, s, "wf__a.yaml@v9.9.9")
		assert.Contains(t, s, "wf__b.yaml@v9.9.9")
	})

	t.Run("source 不正は error", func(t *testing.T) {
		t.Parallel()
		_, err := pinUsesRef([]byte("noop\n"), "invalid", SelfVersion)
		require.Error(t, err)
	})
}
