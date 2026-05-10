// Package configure_go_auth は `flame ci configure-go-auth` subcommand。 GitHub Actions runner 上で private な Go module (`github.com/wakuwaku3/*` 等) を `go build` / `go mod tidy` から取得するため、 git に URL prefix 一致の credential rewrite を `--global` で注入する (FLM_FEA_0004 §責務範囲: CI 補助)。 GH_TOKEN env を入力に取り、 `git config --global url.<token-bearing>.insteadOf https://github.com/` を冪等に実行する。
//
// 本 endpoint は GitHub Actions runner (= 使い切り環境) 専用である。 `git config --global` でホスト全体の設定を上書きし、 同 host で動く他プロセスが当該 token 入り URL rewrite を共有するため、 開発者の local 環境で実行すると個人 gitconfig が token 入りで汚染される。 local 開発では `flame devbox init` の `gh auth setup-git` 経路が gh 経由の credential helper を冪等に登録するため、 本 endpoint を呼ぶ必要は無い。
package configure_go_auth

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

func New() clix.Subcommand {
	return clix.NewLeaf("configure-go-auth", "GitHub Actions runner 上で private な Go module fetch 用に git の URL rewrite を --global に注入する", run)
}

func run(ctx context.Context, in clix.RunInput) error {
	if len(in.Args()) != 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame ci configure-go-auth (GH_TOKEN env required)")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		fmt.Fprintln(in.Stderr(), "error: GH_TOKEN env must be set")
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	rewriteFrom := fmt.Sprintf("url.https://x-access-token:%s@github.com/.insteadOf", token)
	cmd := exec.CommandContext(ctx, "git", "config", "--global", rewriteFrom, "https://github.com/") //nolint:gosec // G204: 起動 binary は固定文字列 "git"。 第 4 引数 rewriteFrom は GH_TOKEN env を含む文字列だが、 git config --global の key として安全に扱われる文字 (英数 + ASCII 記号) のみで構成され、 endpoint の責務として GH_TOKEN を URL rewrite として global gitconfig に書き込むのが意図。
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = in.Stdout()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(in.Stderr(), "error: git config failed: %v: %s\n", err, stderr.String())
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}
