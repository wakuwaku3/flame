package noop_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/ci/noop"
	"github.com/wakuwaku3/flame/lib/clix"
)

// service-level test (FLM_APP_0009): `flame ci noop [reason]` は実体ジョブ起動 0 件時に exit 0 で success log を吐く固定経路。 reason 引数の有無 / 複数 token を空白結合する仕様を service-level でカバーする。
func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("reason なしは default メッセージで exit 0", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(noop.New())
		fake := clix.NewFakeIO(t, []string{"noop"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "noop success: no work to do\n", "")
	})

	t.Run("reason は複数引数を空白で結合して emit する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(noop.New())
		fake := clix.NewFakeIO(t, []string{"noop", "no", "apps", "with", "_tool", "suffix"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "noop success: no apps with _tool suffix\n", "")
	})
}
