// Package path_base は `flame ci label path-base` subcommand。 PR の変更 path から repo root 直下の Go module を特定し、 `module/<name>` ラベルを PR に付与する (FLM_FEA_0003)。 移行元 shell `.github/scripts/apply-pr-labels.sh` を Go ネイティブで再実装する。
package path_base

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const exitCodeFailure = 1

func New() clix.Subcommand {
	return clix.NewLeaf("path-base", "変更 path から module label を PR に付与する", run)
}

func run(ctx context.Context, in clix.RunInput) error {
	return doRun(ctx, in.Stdout(), in.Stderr(), &execOps{})
}

type ghOps interface {
	listLabels(ctx context.Context, repo string) ([]string, error)
	createLabel(ctx context.Context, repo, label, color, description string) error
	addPRLabel(ctx context.Context, repo, prNumber, label string) error
}

type execOps struct{}

func (*execOps) listLabels(ctx context.Context, repo string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "gh", "label", "list", "--repo", repo, "--limit", "200", "--json", "name") //nolint:gosec // G204: gh 起動 + 内部固定 argv (repo は GITHUB_REPOSITORY 由来 envvar、 caller 内部値)。
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, ex.Wrapf(err, "gh label list failed: %s", stderr.String())
	}
	var entries []struct {
		Name string `json:"name"`
	}
	if jsonErr := json.Unmarshal(stdout.Bytes(), &entries); jsonErr != nil {
		return nil, ex.Wrap(jsonErr)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out, nil
}

func (*execOps) createLabel(ctx context.Context, repo, label, color, description string) error {
	cmd := exec.CommandContext(ctx, "gh", "label", "create", label, "--repo", repo, "--color", color, "--description", description) //nolint:gosec // G204: gh 起動 + caller 内部値 (label / color / description は本 endpoint で固定または derived value)。
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return ex.Wrap(cmd.Run())
}

func (*execOps) addPRLabel(ctx context.Context, repo, prNumber, label string) error {
	cmd := exec.CommandContext(ctx, "gh", "pr", "edit", prNumber, "--repo", repo, "--add-label", label) //nolint:gosec // G204: gh 起動 + caller 内部値 (prNumber / label / repo はすべて env / derived 値)。
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return ex.Wrap(cmd.Run())
}

func doRun(ctx context.Context, stdout, stderr io.Writer, op ghOps) error {
	filesJSON, err := requireEnv("FILES_JSON")
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	prNumber, err := requireEnv("PR_NUMBER")
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	repo, err := requireEnv("GITHUB_REPOSITORY")
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if _, tokenErr := requireEnv("GH_TOKEN"); tokenErr != nil {
		fmt.Fprintf(stderr, "%s\n", tokenErr)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}

	var files []string
	if jsonErr := json.Unmarshal([]byte(filesJSON), &files); jsonErr != nil {
		fmt.Fprintf(stderr, "error: FILES_JSON must be a JSON array (got: %s)\n", filesJSON)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if len(files) == 0 {
		fmt.Fprintln(stdout, "no changed files; nothing to label")
		return nil
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	modules, err := listGoModules(repoRoot)
	if err != nil {
		return ex.Wrap(err)
	}
	if len(modules) == 0 {
		fmt.Fprintln(stdout, "no go modules found; nothing to label")
		return nil
	}

	matched := matchModules(modules, files)
	if len(matched) == 0 {
		fmt.Fprintln(stdout, "no module-scoped changes; nothing to label")
		return nil
	}

	existing, err := op.listLabels(ctx, repo)
	if err != nil {
		return ex.Wrap(err)
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		existingSet[n] = struct{}{}
	}

	for _, module := range matched {
		label := "module/" + module
		if _, ok := existingSet[label]; !ok {
			fmt.Fprintf(stdout, "creating label '%s'\n", label)
			if createErr := op.createLabel(ctx, repo, label, "0E8A16", "Changes affecting the "+module+" module"); createErr != nil {
				return ex.Wrap(createErr)
			}
		}
		fmt.Fprintf(stdout, "adding label '%s' to PR #%s\n", label, prNumber)
		if addErr := op.addPRLabel(ctx, repo, prNumber, label); addErr != nil {
			return ex.Wrap(addErr)
		}
	}
	return nil
}

// listGoModules は repo root 直下の go.mod を持つディレクトリ名を辞書順で返す (shell 版 `for module_path in "$repo_root"/*/` と同等)。
func listGoModules(repoRoot string) ([]string, error) {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	out := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(repoRoot, e.Name(), "go.mod")); statErr != nil {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// matchModules は changed_files のうち少なくとも 1 つが `<module>/...` に属する module 名を、 modules の順序を保って返す (shell 版 `matched_modules` の二重ループと同等の意味)。
func matchModules(modules, files []string) []string {
	out := make([]string, 0, len(modules))
	for _, m := range modules {
		prefix := m + "/"
		for _, f := range files {
			if strings.HasPrefix(f, prefix) {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

func requireEnv(name string) (string, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", ex.Errorf("%s must be set", name)
	}
	return v, nil
}
