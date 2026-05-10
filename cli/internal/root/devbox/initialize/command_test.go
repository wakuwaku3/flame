package initialize_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/devbox/initialize"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("flame devbox init は引数なしで起動して副作用なしで成功する (placeholder no-op)", func(t *testing.T) {
		t.Parallel()
		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(initialize.New())
		fake := clix.NewFakeIO(t, []string{"init"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})
}
