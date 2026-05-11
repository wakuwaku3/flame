package install

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wakuwaku3/flame/lib/ex"
)

// vendorGitignoreBlock は初回 install 時に repo root の .gitignore に scaffold する block (FLM_FEA_0003 §利用側 setup 手順)。 vendor 配下は git untracked にする方針へ転換したため `!vendor/flame` は持たない (再取得経路は SyncVendor が clone でカバーする)。
const vendorGitignoreBlock = "# flame harness が install 時に追加する block (FLM_FEA_0003)\ntmp\n.devbox\n.direnv\n.local\n.claude/.ccache\n.claude/scheduled_tasks.lock\nvendor/*\n"

// applyGitignore は repo root の .gitignore に scaffold block を冪等追記する。 既に `vendor/*` 行が含まれていれば no-op。 manifest.ignore に `.gitignore` が含まれていれば no-op (caller 側で skip 判定済を想定)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期的な local file IO のみで cancel 経路を持たない。
func applyGitignore(_ context.Context, repoRoot string) error {
	path := filepath.Join(repoRoot, ".gitignore")
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ex.Wrapf(os.WriteFile(path, []byte(vendorGitignoreBlock), filePerm), "write .gitignore scaffold: %s", path)
	case err != nil:
		return ex.Wrapf(err, "read .gitignore: %s", path)
	}
	if hasVendorIgnoreBlock(existing) {
		return nil
	}
	updated := appendGitignoreBlock(existing)
	if err := os.WriteFile(path, updated, filePerm); err != nil {
		return ex.Wrapf(err, "write .gitignore: %s", path)
	}
	return nil
}

func hasVendorIgnoreBlock(content []byte) bool {
	for line := range strings.SplitSeq(string(content), "\n") {
		if strings.TrimSpace(line) == "vendor/*" {
			return true
		}
	}
	return false
}

func appendGitignoreBlock(existing []byte) []byte {
	out := append([]byte(nil), existing...)
	if len(out) > 0 && !strings.HasSuffix(string(out), "\n") {
		out = append(out, '\n')
	}
	if len(out) > 0 {
		out = append(out, '\n')
	}
	out = append(out, []byte(vendorGitignoreBlock)...)
	return out
}
