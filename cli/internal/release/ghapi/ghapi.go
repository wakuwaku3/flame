// Package ghapi は gh CLI 経由の GitHub REST 呼び出しを 1 か所に集約する thin abstraction (FLM_FEA_0004 §リリースノート / §版番号の決定経路 が消費する compare / pulls / releases endpoint をまとめて扱うため)。 production 実装は `gh` バイナリを exec し、 test では Stub に差し替えて network 依存を排除する。
package ghapi

import (
	"bytes"
	"context"
	"errors"
	"os/exec"

	"github.com/wakuwaku3/flame/lib/ex"
)

// Client は gh CLI 経由の REST 呼び出し interface。 Production は Exec、 test は Stub を使う。
type Client interface {
	API(ctx context.Context, path string) ([]byte, error)
	ReleaseDownload(ctx context.Context, tag, pattern, outPath string) error
}

// Exec は `gh` バイナリを exec する production 実装。 PATH 解決は OS に委ね、 test では fake gh を PATH 先頭に置いて injection する経路を持つ。
type Exec struct{}

var _ Client = (*Exec)(nil)

// API は `gh api --paginate -H "Accept: application/vnd.github+json" <path>` 相当の出力を返す。 path は `/repos/<owner>/<repo>/...` 等の REST endpoint 直値。
func (Exec) API(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", "api", "--paginate", "-H", "Accept: application/vnd.github+json", path) //nolint:gosec // G204: gh 起動 + flame CLI 内部組立 path のみ。 path は呼び出し元で REST endpoint 文字列として固定。
	return runOutput(cmd)
}

// ReleaseDownload は `gh release download <tag> --pattern <pattern> --output <outPath> --clobber` 相当。 tag が無い / pattern にマッチする asset が無いとき非 nil error を返す (caller は warn ログに留めて patch bump fallback する経路)。
func (Exec) ReleaseDownload(ctx context.Context, tag, pattern, outPath string) error {
	cmd := exec.CommandContext(ctx, "gh", "release", "download", tag, "--pattern", pattern, "--output", outPath, "--clobber") //nolint:gosec // G204: gh 起動 + flame CLI 内部組立 argv のみ。
	_, err := runOutput(cmd)
	return err
}

func runOutput(cmd *exec.Cmd) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, ex.Wrapf(err, "gh failed: %s", stderr.String())
		}
		return nil, ex.Wrap(err)
	}
	return stdout.Bytes(), nil
}
