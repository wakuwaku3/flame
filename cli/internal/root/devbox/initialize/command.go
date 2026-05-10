// Package initialize は `flame devbox init` subcommand。 devbox shell の init 経路で flame CLI 自身が必要とする repo 固有の追加 setup (= 将来的な拡張点) を担う placeholder。 現状は副作用なしの no-op。 flame CLI 自身の install は本 subcommand では扱わず devbox.json の init_hook で完結させる (FLI_FEA_0002 §責務範囲: devbox 補助 / 本 subcommand は flame install 後に走るため flame install を担うことができない)。
package initialize

import (
	"context"

	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	return clix.NewLeaf("init", "devbox 環境を初期化する (placeholder。 現状は副作用なし)", run)
}

func run(_ context.Context, _ clix.RunInput) error {
	return nil
}
