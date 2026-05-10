package detect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const exitCodeUsage = 2

func New() clix.Subcommand {
	return clix.NewLeaf("detect", "配布対象 _tool バイナリを enumerate する", run)
}

type appEntry struct {
	AppName string `json:"app_name"`
	Module  string `json:"module"`
	AppDir  string `json:"app_dir"`
}

type appsMatrix struct {
	Apps []appEntry `json:"apps"`
}

func run(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) != 1 {
		fmt.Fprintln(in.Stderr(), "usage: flame ci deploy detect <github_output_path>")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	githubOutput := args[0]
	repoRoot, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	apps, err := enumerateApps(repoRoot)
	if err != nil {
		return ex.Wrap(err)
	}
	hasApps := len(apps) > 0
	matrixJSON, err := marshalMatrix(apps)
	if err != nil {
		return ex.Wrap(err)
	}
	return ex.Wrap(emitOutputs(in.Stdout(), githubOutput,
		fmt.Sprintf("matrix=%s", matrixJSON),
		fmt.Sprintf("has_apps=%t", hasApps),
	))
}

// marshalMatrix は配布対象 0 件時に shell 互換の `{"apps":[]}` を emit するため、 nil slice ではなく空 slice を保証する経路を経由する (Go の json は nil slice を `null` にする)。
func marshalMatrix(apps []appEntry) ([]byte, error) {
	matrix := appsMatrix{Apps: apps}
	if matrix.Apps == nil {
		matrix.Apps = []appEntry{}
	}
	out, err := json.Marshal(matrix)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	return out, nil
}

// enumerateApps は repo_root 直下の各 module (= go.mod を持つ ディレクトリ) の cmd/ 配下から `_tool` suffix を持つ app_dir を集める (FLM_APP_0007 §配置)。 出力順は shell 版の glob 走査順 (= 辞書順) と揃える。
func enumerateApps(repoRoot string) ([]appEntry, error) {
	moduleNames, err := listSortedDirs(repoRoot)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	out := make([]appEntry, 0)
	for _, moduleName := range moduleNames {
		moduleDir := filepath.Join(repoRoot, moduleName)
		if _, statErr := os.Stat(filepath.Join(moduleDir, "go.mod")); statErr != nil {
			continue
		}
		appNames, readErr := listSortedDirs(filepath.Join(moduleDir, "cmd"))
		if readErr != nil {
			continue
		}
		for _, appDir := range appNames {
			if !strings.HasSuffix(appDir, "_tool") {
				continue
			}
			out = append(out, appEntry{
				AppName: strings.TrimSuffix(appDir, "_tool"),
				Module:  moduleName,
				AppDir:  appDir,
			})
		}
	}
	return out, nil
}

func listSortedDirs(parent string) ([]string, error) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func emitOutputs(stdout io.Writer, githubOutput string, lines ...string) error {
	f, err := os.OpenFile(githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fsperm.File) //nolint:gosec // G304: githubOutput は CI runner が GITHUB_OUTPUT に書き出す既知 path で、 当該 step output を append するのが本 endpoint の責務。
	if err != nil {
		return ex.Wrap(err)
	}
	defer f.Close() //nolint:errcheck // append-only step output への close 失敗時に意味ある復旧経路は無く、 後続書込みも無いため握り潰す。
	for _, line := range lines {
		if _, writeErr := fmt.Fprintln(stdout, line); writeErr != nil {
			return ex.Wrap(writeErr)
		}
		if _, writeErr := fmt.Fprintln(f, line); writeErr != nil {
			return ex.Wrap(writeErr)
		}
	}
	return nil
}
