package detect_lib

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
	return clix.NewLeaf("detect-lib", "配布対象 library module を enumerate する", run)
}

type libEntry struct {
	Module     string `json:"module"`
	ModulePath string `json:"module_path"`
}

type libsMatrix struct {
	Libs []libEntry `json:"libs"`
}

func run(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) != 1 {
		fmt.Fprintln(in.Stderr(), "usage: flame ci deploy detect-lib <github_output_path>")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	githubOutput := args[0]
	repoRoot, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	libs, err := enumerateLibs(repoRoot)
	if err != nil {
		return ex.Wrap(err)
	}
	hasLibs := len(libs) > 0
	if libs == nil {
		libs = []libEntry{}
	}
	matrixJSON, err := json.Marshal(libsMatrix{Libs: libs})
	if err != nil {
		return ex.Wrap(err)
	}
	return ex.Wrap(emitOutputs(in.Stdout(), githubOutput,
		fmt.Sprintf("matrix=%s", matrixJSON),
		fmt.Sprintf("has_libs=%t", hasLibs),
	))
}

// enumerateLibs は repo_root 直下の各 module (= go.mod を持つディレクトリ) のうち module 名が `lib` または `*_lib` suffix を持つものを集める (FLM_APP_0007 §配置)。 出力順は shell の glob 走査順 (= 辞書順) に揃える。
func enumerateLibs(repoRoot string) ([]libEntry, error) {
	entries, err := os.ReadDir(repoRoot)
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

	out := make([]libEntry, 0)
	for _, moduleName := range names {
		moduleDir := filepath.Join(repoRoot, moduleName)
		if _, statErr := os.Stat(filepath.Join(moduleDir, "go.mod")); statErr != nil {
			continue
		}
		if moduleName != "lib" && !strings.HasSuffix(moduleName, "_lib") {
			continue
		}
		out = append(out, libEntry{Module: moduleName, ModulePath: moduleName})
	}
	return out, nil
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
