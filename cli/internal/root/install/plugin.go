package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wakuwaku3/flame/lib/ex"
)

// applyPluginMarketplace は Claude Code の plugin marketplace 登録 + plugin install を `.claude/settings.json` に対して書き込む (FLM_FEA_0003 §チャネル A)。 manifest の `flame.source` (= `github.com/<owner>/<repo>`) から marketplace を組み立て、 plugin 名は marketplace 末尾の repo 名と同一 (= `<repo>@<repo>`) とする。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期的な local file IO のみで cancel 経路を持たない。
func applyPluginMarketplace(ctx context.Context, repoRoot, source string) error {
	owner, repo, err := parseSource(source)
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(repoRoot, ".claude", "settings.json")
	settings, err := readSettings(ctx, settingsPath)
	if err != nil {
		return err
	}
	if err := mergeMarketplace(settings, repo, owner); err != nil {
		return err
	}
	mergeEnabledPlugin(settings, repo)
	return writeSettings(ctx, settingsPath, settings)
}

const sourceParts = 2

func parseSource(source string) (owner, repo string, err error) {
	trimmed := strings.TrimPrefix(source, "github.com/")
	parts := strings.SplitN(trimmed, "/", sourceParts)
	if len(parts) != sourceParts || parts[0] == "" || parts[1] == "" {
		return "", "", ex.Errorf("flame.source must be `github.com/<owner>/<repo>`: %q", source)
	}
	return parts[0], parts[1], nil
}

func readSettings(_ context.Context, path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, ex.Wrapf(err, "read settings: %s", path)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, ex.Wrapf(err, "parse settings: %s", path)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func mergeMarketplace(settings map[string]any, repo, owner string) error {
	raw, ok := settings["extraKnownMarketplaces"]
	marketplaces := map[string]any{}
	if ok {
		typed, typeOK := raw.(map[string]any)
		if !typeOK {
			return ex.Errorf("settings.json: extraKnownMarketplaces must be an object")
		}
		marketplaces = typed
	}
	marketplaces[repo] = map[string]any{
		"source": map[string]any{
			"source": "github",
			"repo":   fmt.Sprintf("%s/%s", owner, repo),
		},
	}
	settings["extraKnownMarketplaces"] = marketplaces
	return nil
}

func mergeEnabledPlugin(settings map[string]any, repo string) {
	raw, ok := settings["enabledPlugins"]
	enabled := map[string]any{}
	if ok {
		typed, typeOK := raw.(map[string]any)
		if typeOK {
			enabled = typed
		}
	}
	key := fmt.Sprintf("%s@%s", repo, repo)
	enabled[key] = true
	settings["enabledPlugins"] = enabled
}

func writeSettings(_ context.Context, path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return ex.Wrapf(err, "mkdir settings parent: %s", path)
	}
	body, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return ex.Wrap(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, filePerm); err != nil {
		return ex.Wrapf(err, "write settings: %s", path)
	}
	return nil
}
