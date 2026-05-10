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

// applyEmbed は repo root の取り込み形式 file (CLAUDE.md / .envrc / .yamllint) に embed snippet が含まれていることを保証する。 file が無ければ snippet 単体で scaffold する。 既に snippet が含まれていれば no-op。 含まれていなければ末尾に追記する。 戻り値の LockEmbed は flame.lock.embeds[] への entry。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) で受け取るが本処理は同期 file IO のみ。
func applyEmbed(_ context.Context, repoRoot string, item PlanItem) (LockEmbed, error) {
	rule, ok := lookupEmbedRule(item.VendorPath)
	if !ok {
		return LockEmbed{Install: "", Target: "", Snippet: ""}, ex.Errorf("no embed rule for %s", item.VendorPath)
	}
	installAbs := filepath.Join(repoRoot, item.InstallPath)
	existing, err := os.ReadFile(installAbs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if writeErr := writeEmbedScaffold(installAbs, rule.Snippet); writeErr != nil {
			return LockEmbed{Install: "", Target: "", Snippet: ""}, writeErr
		}
	case err != nil:
		return LockEmbed{Install: "", Target: "", Snippet: ""}, ex.Wrapf(err, "read embed install file: %s", installAbs)
	default:
		if !containsSnippet(existing, rule.Snippet) {
			updated := appendSnippet(existing, rule.Snippet)
			if writeErr := os.WriteFile(installAbs, updated, filePerm); writeErr != nil {
				return LockEmbed{Install: "", Target: "", Snippet: ""}, ex.Wrapf(writeErr, "write embed install file: %s", installAbs)
			}
		}
	}
	return LockEmbed{
		Install: filepath.ToSlash(item.InstallPath),
		Target:  filepath.ToSlash(item.VendorPath),
		Snippet: rule.Snippet,
	}, nil
}

func lookupEmbedRule(vendorPath string) (EmbedRule, bool) {
	want := filepath.ToSlash(vendorPath)
	for _, r := range embedRules {
		if filepath.ToSlash(r.VendorPath) == want {
			return r, true
		}
	}
	return EmbedRule{VendorPath: "", Snippet: ""}, false
}

func writeEmbedScaffold(path, snippet string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return ex.Wrapf(err, "mkdir embed parent: %s", path)
	}
	if err := os.WriteFile(path, []byte(snippet), filePerm); err != nil {
		return ex.Wrapf(err, "write embed scaffold: %s", path)
	}
	return nil
}

func containsSnippet(content []byte, snippet string) bool {
	return strings.Contains(string(content), strings.TrimRight(snippet, "\n"))
}

func appendSnippet(content []byte, snippet string) []byte {
	out := append([]byte(nil), content...)
	if len(out) > 0 && !strings.HasSuffix(string(out), "\n") {
		out = append(out, '\n')
	}
	out = append(out, []byte(snippet)...)
	return out
}
