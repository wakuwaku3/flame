// Package noop は `flame ci noop [reason]` subcommand。 検査対象 / 配布対象 0 件で実体ジョブを起動できないケースで、 success の旨を CI ログに残して exit 0 で終了する固定 success 用 endpoint (FLM_ENG_0003 §並列化: 必ず 1 件以上の success job を返す / FLM_FEA_0002 §summary)。
package noop

import (
	"context"
	"fmt"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
)

func New() clix.Subcommand {
	return clix.NewLeaf("noop", "実行対象 0 件時の success log を stdout に出して exit 0 を返す", run)
}

func run(_ context.Context, in clix.RunInput) error {
	reason := strings.Join(in.Args(), " ")
	if reason == "" {
		reason = "no work to do"
	}
	fmt.Fprintf(in.Stdout(), "noop success: %s\n", reason)
	return nil
}
