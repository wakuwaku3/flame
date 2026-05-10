// Package lib は `flame ci release lib <module> <commit_sha>` subcommand。 配布対象 library module の release を end-to-end で実行する orchestrator (FLM_FEA_0002 §library 配布)。 移行元 shell `.github/scripts/deploy/release-lib.sh` と同一の振る舞いを Go ネイティブで再実装する。
package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/release/ghapi"
	"github.com/wakuwaku3/flame/cli/internal/release/notes"
	"github.com/wakuwaku3/flame/cli/internal/release/version"
	speclib "github.com/wakuwaku3/flame/cli/internal/root/ci/release/spec/lib"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage     = 2
	exitCodeFailure   = 1
	specAssetName     = "lib-spec.json"
	requiredArgsCount = 2
)

func New() clix.Subcommand {
	return clix.NewLeaf("lib", "配布対象 library module を release する", run)
}

func run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) != requiredArgsCount {
		fmt.Fprintln(in.Stderr(), "usage: flame ci release lib <module> <commit_sha>")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	return doRun(ctx, in.Stdout(), in.Stderr(), args[0], args[1], ghapi.Exec{}, &execSystemOps{})
}

// systemOps は git / gh への副作用を test 内で fake に差し替える fault boundary (FLM_APP_0009 §mock を採用しない / fake を採用する)。 production は execSystemOps、 test は test 内に置く fake を渡す。
type systemOps interface {
	configureGitIdentity(ctx context.Context, actor string) error
	tagExistsLocal(ctx context.Context, repoRoot, tag string) bool
	tagExistsRemote(ctx context.Context, repoRoot, tag string) bool
	gitTagAndPush(ctx context.Context, repoRoot, tag, commitSHA string) error
	ghReleaseCreate(ctx context.Context, tag, title, notesPath, commitSHA string, assets []string) error
}

type execSystemOps struct{}

func (*execSystemOps) configureGitIdentity(ctx context.Context, actor string) error {
	if err := runForeground(ctx, "", "git", "config", "--global", "user.name", actor); err != nil {
		return ex.Wrap(err)
	}
	return ex.Wrap(runForeground(ctx, "", "git", "config", "--global", "user.email", actor+"@users.noreply.github.com"))
}

func (*execSystemOps) tagExistsLocal(ctx context.Context, repoRoot, tag string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "-q", "--verify", "refs/tags/"+tag) //nolint:gosec // G204: git 起動 + tag 引数 (内部固定 prefix `refs/tags/` + caller 内部値) は外部入力ではない。
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

func (*execSystemOps) tagExistsRemote(ctx context.Context, repoRoot, tag string) bool {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", "--tags", "origin", "refs/tags/"+tag) //nolint:gosec // G204: 同上、 tag は内部値。
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

func (*execSystemOps) gitTagAndPush(ctx context.Context, repoRoot, tag, commitSHA string) error {
	if err := runForeground(ctx, repoRoot, "git", "tag", tag, commitSHA); err != nil {
		return ex.Wrap(err)
	}
	return ex.Wrap(runForeground(ctx, repoRoot, "git", "push", "origin", "refs/tags/"+tag))
}

func (*execSystemOps) ghReleaseCreate(ctx context.Context, tag, title, notesPath, commitSHA string, assets []string) error {
	const fixedArgsCount = 9 // "release" "create" tag "--target" commitSHA "--title" title "--notes-file" notesPath
	args := make([]string, 0, len(assets)+fixedArgsCount)
	args = append(args, "release", "create", tag, "--target", commitSHA, "--title", title, "--notes-file", notesPath)
	args = append(args, assets...)
	return ex.Wrap(runForeground(ctx, "", "gh", args...))
}

func runForeground(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: caller (release/lib のみ) が固定 binary 名 + 内部組立 argv を渡す経路で外部入力ではない。
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run() //nolint:wrapcheck // helper 内部の生 error。 caller (doRun) が ex.Wrap 済みの contextual error を返す。
}

func doRun(ctx context.Context, stdout, stderr io.Writer, module, commitSHA string, gh ghapi.Client, sys systemOps) error {
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		fmt.Fprintln(stderr, "GITHUB_REPOSITORY must be set")
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	dryRun := os.Getenv("DRY_RUN") == "true"
	actor := os.Getenv("ACTOR")
	if !dryRun && actor == "" {
		fmt.Fprintln(stderr, "ACTOR must be set")
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	priorOverride := os.Getenv("PRIOR_TAG_OVERRIDE")

	repoRoot, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	moduleDir := filepath.Join(repoRoot, module)
	importPath, err := readModuleImportPath(moduleDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}

	workDir, err := os.MkdirTemp("", "flame-release-lib-")
	if err != nil {
		return ex.Wrap(err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck // tmp dir 後始末 best-effort。

	specPath := filepath.Join(workDir, specAssetName)
	specOut, err := os.Create(specPath) //nolint:gosec // G304: workDir 配下の固定 path への書き出し。
	if err != nil {
		return ex.Wrap(err)
	}
	if extractErr := speclib.EmitTo(moduleDir, specOut); extractErr != nil {
		_ = specOut.Close()
		return ex.Wrap(extractErr)
	}
	if closeErr := specOut.Close(); closeErr != nil {
		return ex.Wrap(closeErr)
	}

	plan, err := version.Compute(ctx, stderr, &version.Input{
		GH:               gh,
		Flatten:          flattenLibSpec,
		Repo:             repo,
		TagPrefix:        module + "/v",
		NewSpecPath:      specPath,
		WorkDir:          workDir,
		PriorTagOverride: priorOverride,
		SpecAssetName:    specAssetName,
		ForbidMajor:      true,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}

	priorDisplay := plan.Prior
	if priorDisplay == "" {
		priorDisplay = "<none>"
	}
	fmt.Fprintf(stdout, "release plan: module=%s import=%s prior=%s bump=%s next=%s dry_run=%t\n",
		module, importPath, priorDisplay, plan.Bump, plan.Next, dryRun)

	priorTag := ""
	if plan.Prior != "" {
		priorTag = module + "/v" + plan.Prior
	}

	// FLM_FEA_0002 §リリース起動契機: 前回 release tag → 今回 commit で当該 module ディレクトリ配下に file change が 1 件も無い場合は release を作らない。 初版 release (priorTag 無し) は変更検査なしで作成する。 skip 判定は configureGitIdentity / notes 生成より前に行うことで、 skip 時の副作用 (git config 書き換え / notes file 作成) も避ける。
	if priorTag != "" && !notes.HasModuleChangesSincePriorTag(ctx, stderr, gh, repo, priorTag, commitSHA, module) {
		return ex.Wrap(emitSkipSummary(stdout, "Library", module, module, priorTag, commitSHA))
	}

	if !dryRun {
		if cfgErr := sys.configureGitIdentity(ctx, actor); cfgErr != nil {
			return ex.Wrap(cfgErr)
		}
	}
	notesPath := filepath.Join(workDir, "release-notes.md")
	notesFile, err := os.Create(notesPath) //nolint:gosec // G304: workDir 配下の固定 path。
	if err != nil {
		return ex.Wrap(err)
	}
	composeErr := notes.Compose(ctx, notesFile, stderr, &notes.ComposeInput{
		GHAPI:          gh,
		Repo:           repo,
		ModuleName:     module,
		Heading:        module,
		CommitSHA:      commitSHA,
		PriorTag:       priorTag,
		InstallSnippet: fmt.Sprintf("go get %s@v%s", importPath, plan.Next),
	})
	closeErr := notesFile.Close()
	if composeErr != nil {
		return ex.Wrap(composeErr)
	}
	if closeErr != nil {
		return ex.Wrap(closeErr)
	}

	tag := module + "/v" + plan.Next
	title := module + " v" + plan.Next

	if dryRun {
		return ex.Wrap(emitDryRunPreview(stdout, "Library", module, plan.Next, priorTag, commitSHA, notesPath, tag))
	}

	if sys.tagExistsLocal(ctx, repoRoot, tag) {
		fmt.Fprintf(stderr, "error: tag '%s' already exists locally\n", tag)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if sys.tagExistsRemote(ctx, repoRoot, tag) {
		fmt.Fprintf(stderr, "error: tag '%s' already exists on origin\n", tag)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if pushErr := sys.gitTagAndPush(ctx, repoRoot, tag, commitSHA); pushErr != nil {
		return ex.Wrap(pushErr)
	}
	return ex.Wrap(sys.ghReleaseCreate(ctx, tag, title, notesPath, commitSHA, []string{specPath}))
}

func readModuleImportPath(moduleDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(moduleDir, "go.mod")) //nolint:gosec // G304: 配布対象 library module の go.mod 読み出し。
	if err != nil {
		return "", ex.Wrap(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", ex.Errorf("no `module` directive found in %s/go.mod", moduleDir)
}

// emitDryRunPreview の `nextVersion` は version package との import shadow を避ける目的で `version` を避けた命名 (gocritic importShadow)。
func emitDryRunPreview(stdout io.Writer, kind, name, nextVersion, priorTag, commitSHA, notesPath, tag string) error {
	fmt.Fprintf(stdout, "DRY RUN: skipping tag push and gh release create for %s\n", tag)
	fmt.Fprintln(stdout, "=== release notes preview (DRY RUN) ===")
	notesBytes, err := os.ReadFile(notesPath) //nolint:gosec // G304: workDir 配下の compose 済 notes。
	if err != nil {
		return ex.Wrap(err)
	}
	if _, writeErr := stdout.Write(notesBytes); writeErr != nil {
		return ex.Wrap(writeErr)
	}
	stepSummary := os.Getenv("GITHUB_STEP_SUMMARY")
	if stepSummary != "" {
		summary, openErr := os.OpenFile(stepSummary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fsperm.File) //nolint:gosec // G304: GitHub Actions の既知 path への append。
		if openErr != nil {
			return ex.Wrap(openErr)
		}
		defer summary.Close() //nolint:errcheck // append-only summary。
		priorDisplay := priorTag
		if priorDisplay == "" {
			priorDisplay = "<none>"
		}
		fmt.Fprintln(summary, "## Dry-run release notes preview")
		fmt.Fprintln(summary)
		fmt.Fprintf(summary, "**%s:** `%s` v%s (prior=`%s`, head=`%s`)\n", kind, name, nextVersion, priorDisplay, commitSHA)
		fmt.Fprintln(summary)
		fmt.Fprintln(summary, "```markdown")
		_, _ = summary.Write(notesBytes)
		fmt.Fprintln(summary, "```")
	}
	return nil
}

// emitSkipSummary は release skip 時の stdout / GITHUB_STEP_SUMMARY 出力を担当する (FLM_FEA_0002 §リリース起動契機)。 stdout は workflow ログ用の 1 行、 GITHUB_STEP_SUMMARY は Actions UI の Job summary 上に Release skipped セクションを残す。 modulePath は file change 判定に使った module dir prefix で、 表示上の name と別 (tool 系統だと name=appName / modulePath=module で別物)。
func emitSkipSummary(stdout io.Writer, kind, name, modulePath, priorTag, commitSHA string) error {
	fmt.Fprintf(stdout, "skip release: no file changes under %s/ since %s (FLM_FEA_0002 §リリース起動契機)\n", modulePath, priorTag)
	stepSummary := os.Getenv("GITHUB_STEP_SUMMARY")
	if stepSummary == "" {
		return nil
	}
	summary, err := os.OpenFile(stepSummary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fsperm.File) //nolint:gosec // G304: GitHub Actions の既知 path への append。
	if err != nil {
		return ex.Wrap(err)
	}
	defer summary.Close() //nolint:errcheck // append-only summary。
	fmt.Fprintln(summary, "## Release skipped")
	fmt.Fprintln(summary)
	fmt.Fprintf(summary, "**%s:** `%s` (prior=`%s`, head=`%s`)\n", kind, name, priorTag, commitSHA)
	fmt.Fprintln(summary)
	fmt.Fprintf(summary, "No file changes under `%s/` since `%s`.\n", modulePath, priorTag)
	return nil
}

func flattenLibSpec(specJSON []byte) (map[string]string, error) {
	type identifier struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Signature string `json:"signature"`
	}
	type packageSpec struct {
		Path        string       `json:"path"`
		Identifiers []identifier `json:"identifiers"`
	}
	type librarySpec struct {
		Packages []packageSpec `json:"packages"`
	}
	var spec librarySpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, ex.Wrap(err)
	}
	out := make(map[string]string)
	for _, p := range spec.Packages {
		for _, id := range p.Identifiers {
			out[p.Path+"::"+id.Kind+"::"+id.Name] = canonicalizeTypeSignature(id.Kind, id.Signature)
		}
	}
	return out, nil
}

// canonicalizeTypeSignature は spec emitter 切替前 release で発番された baseline (= type 本体に AST 全体を詰めた full body 形式) と現行 emitter (= bare "struct" / "interface" + field / method atomic identifier 形式) の両方を同じ key で diff できるように、 比較前の signature を bare 形式に正規化する。 これが無いと baseline 側の type 本体 signature が format 違いだけで shape_changed になり、 単純な field / method 追加でも MAJOR 誤検出する。
func canonicalizeTypeSignature(kind, signature string) string {
	if kind != "type" {
		return signature
	}
	switch {
	case signature == "struct" || strings.HasPrefix(signature, "struct ") || strings.HasPrefix(signature, "struct{"):
		return "struct"
	case signature == "interface" || strings.HasPrefix(signature, "interface ") || strings.HasPrefix(signature, "interface{"):
		return "interface"
	default:
		return signature
	}
}
