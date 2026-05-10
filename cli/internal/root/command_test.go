package root_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestExecute(t *testing.T) {
	t.Parallel()

	t.Run("未定義の subcommand で error が返る", func(t *testing.T) {
		t.Parallel()

		// Arrange
		fake := clix.NewFakeIO(t, []string{"definitely-not-a-real-subcommand"})

		// Act
		err := root.Execute(t.Context(), fake)

		// Assert
		require.Error(t, err)
		// cobra root は SilenceErrors=true / SilenceUsage=true 構成で unknown command 時の error 文字列・usage を stdout / stderr に書かない (FLM_APP_0009 §assertion 規約)。
		fake.Verify(t, "", "")
	})
}
