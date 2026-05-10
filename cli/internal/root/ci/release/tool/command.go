// Package tool は `flame ci release tool <module> <app_dir> <app_name> <commit_sha>` subcommand。 配布対象 _tool バイナリの release を end-to-end で実行する orchestrator (FLM_FEA_0002 §tool 配布)。 移行元 shell `.github/scripts/deploy/release-app.sh` と同一の振る舞いを Go ネイティブで再実装する。
package tool

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/release/ghapi"
	"github.com/wakuwaku3/flame/cli/internal/release/notes"
	"github.com/wakuwaku3/flame/cli/internal/release/version"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage     = 2
	exitCodeFailure   = 1
	specAssetName     = "cli-spec.json"
	requiredArgsCount = 4
)

func New() clix.Subcommand {
	return clix.NewLeaf("tool", "配布対象 _tool バイナリを release する", run)
}

func run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) != requiredArgsCount {
		fmt.Fprintln(in.Stderr(), "usage: flame ci release tool <module> <app_dir> <app_name> <commit_sha>")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	return doRun(ctx, in.Stdout(), in.Stderr(), args[0], args[1], args[2], args[3], ghapi.Exec{}, &execSystemOps{})
}

type systemOps interface {
	configureGitIdentity(ctx context.Context, actor string) error
	tagExistsLocal(ctx context.Context, repoRoot, tag string) bool
	tagExistsRemote(ctx context.Context, repoRoot, tag string) bool
	gitTagAndPush(ctx context.Context, repoRoot, tag, commitSHA string) error
	ghReleaseCreate(ctx context.Context, tag, title, notesPath, commitSHA string, assets []string) error
	buildBinary(ctx context.Context, moduleDir, appDir, outPath string, env []string) error
	emitSpec(ctx context.Context, binaryPath, outPath string) error
}

type execSystemOps struct{}

func (*execSystemOps) configureGitIdentity(ctx context.Context, actor string) error {
	if err := runForeground(ctx, "", "git", "config", "--global", "user.name", actor); err != nil {
		return ex.Wrap(err)
	}
	return ex.Wrap(runForeground(ctx, "", "git", "config", "--global", "user.email", actor+"@users.noreply.github.com"))
}

func (*execSystemOps) tagExistsLocal(ctx context.Context, repoRoot, tag string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "-q", "--verify", "refs/tags/"+tag) //nolint:gosec // G204: git 起動 + 内部固定 prefix `refs/tags/` + caller 内部 tag。
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

func (*execSystemOps) buildBinary(ctx context.Context, moduleDir, appDir, outPath string, env []string) error {
	args := []string{"build", "-trimpath", "-o", outPath, "./cmd/" + appDir}
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // G204: go 起動 + 内部組立 argv (outPath / appDir は workDir 配下と caller 内部値)。
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return ex.Wrap(cmd.Run())
}

func (*execSystemOps) emitSpec(ctx context.Context, binaryPath, outPath string) error {
	out, err := os.Create(outPath) //nolint:gosec // G304: workDir 配下の固定 path への書き出し。
	if err != nil {
		return ex.Wrap(err)
	}
	defer out.Close()                                     //nolint:errcheck // append-only。
	cmd := exec.CommandContext(ctx, binaryPath, "__spec") //nolint:gosec // G204: workDir 配下に直前 build した binary。
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	return ex.Wrap(cmd.Run())
}

// runForeground は env 引数を取らない単純化版。 build 系統が env を渡す経路は buildBinary に閉じているため、 release/{git,gh} 起動はすべて process env で十分 (unparam 排除)。
func runForeground(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: 固定 binary 名 + 内部組立 argv。
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run() //nolint:wrapcheck // helper 内部の生 error。
}

func doRun(ctx context.Context, stdout, stderr io.Writer, module, appDir, appName, commitSHA string, gh ghapi.Client, sys systemOps) error {
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

	workDir, err := os.MkdirTemp("", "flame-release-tool-")
	if err != nil {
		return ex.Wrap(err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck // tmp dir 後始末 best-effort。

	binaryPath := filepath.Join(workDir, appName)
	if buildErr := sys.buildBinary(ctx, moduleDir, appDir, binaryPath, nil); buildErr != nil {
		return ex.Wrap(buildErr)
	}
	specPath := filepath.Join(workDir, specAssetName)
	if specErr := sys.emitSpec(ctx, binaryPath, specPath); specErr != nil {
		return ex.Wrap(specErr)
	}

	plan, err := version.Compute(ctx, stderr, &version.Input{
		GH:               gh,
		Repo:             repo,
		TagPrefix:        appName + "/v",
		NewSpecPath:      specPath,
		WorkDir:          workDir,
		SpecAssetName:    specAssetName,
		Flatten:          flattenToolSpec,
		PriorTagOverride: priorOverride,
		ForbidMajor:      false,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}

	priorDisplay := plan.Prior
	if priorDisplay == "" {
		priorDisplay = "<none>"
	}
	fmt.Fprintf(stdout, "release plan: app=%s prior=%s bump=%s next=%s dry_run=%t\n",
		appName, priorDisplay, plan.Bump, plan.Next, dryRun)

	priorTag := ""
	if plan.Prior != "" {
		priorTag = appName + "/v" + plan.Prior
	}

	// FLM_FEA_0002 §リリース起動契機: 前回 release tag → 今回 commit で当該 module ディレクトリ配下に file change が 1 件も無い場合は release を作らない。 初版 release (priorTag 無し) は変更検査なしで作成する。
	if priorTag != "" && !notes.HasModuleChangesSincePriorTag(ctx, stderr, gh, repo, priorTag, commitSHA, module) {
		return ex.Wrap(emitSkipSummary(stdout, "App", appName, module, priorTag, commitSHA))
	}

	if !dryRun {
		if cfgErr := sys.configureGitIdentity(ctx, actor); cfgErr != nil {
			return ex.Wrap(cfgErr)
		}
	}
	notesPath := filepath.Join(workDir, "release-notes.md")
	notesFile, err := os.Create(notesPath) //nolint:gosec // G304: workDir 配下。
	if err != nil {
		return ex.Wrap(err)
	}
	composeErr := notes.Compose(ctx, notesFile, stderr, &notes.ComposeInput{
		GHAPI:          gh,
		Repo:           repo,
		ModuleName:     module,
		Heading:        appName,
		CommitSHA:      commitSHA,
		PriorTag:       priorTag,
		InstallSnippet: fmt.Sprintf("curl -fsSL https://raw.githubusercontent.com/%s/main/%s/scripts/install.sh | bash -s -- %s", repo, module, plan.Next),
	})
	closeErr := notesFile.Close()
	if composeErr != nil {
		return ex.Wrap(composeErr)
	}
	if closeErr != nil {
		return ex.Wrap(closeErr)
	}

	tag := appName + "/v" + plan.Next
	title := appName + " v" + plan.Next

	if dryRun {
		return ex.Wrap(emitDryRunPreview(stdout, "App", appName, plan.Next, priorTag, commitSHA, notesPath, tag))
	}

	if sys.tagExistsLocal(ctx, repoRoot, tag) {
		fmt.Fprintf(stderr, "error: tag '%s' already exists locally\n", tag)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if sys.tagExistsRemote(ctx, repoRoot, tag) {
		fmt.Fprintf(stderr, "error: tag '%s' already exists on origin\n", tag)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if err := sys.gitTagAndPush(ctx, repoRoot, tag, commitSHA); err != nil {
		return ex.Wrap(err)
	}

	assetsDir := filepath.Join(workDir, "assets")
	if mkErr := os.MkdirAll(assetsDir, fsperm.Dir); mkErr != nil {
		return ex.Wrap(mkErr)
	}
	assets, buildErr := buildCrossArchives(ctx, stdout, stderr, sys, moduleDir, module, appDir, appName, plan.Next, assetsDir)
	if buildErr != nil {
		return ex.Wrap(buildErr)
	}
	assets = append(assets, specPath)
	return ex.Wrap(sys.ghReleaseCreate(ctx, tag, title, notesPath, commitSHA, assets))
}

// targetPlatforms は配布対象 OS / arch (FLM_FEA_0002 §配布プラットフォーム)。
var targetPlatforms = []struct{ os, arch string }{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// buildCrossArchives の `nextVersion` は version package との import shadow を避けた命名 (gocritic importShadow)。
func buildCrossArchives(ctx context.Context, stdout, stderr io.Writer, sys systemOps, moduleDir, module, appDir, appName, nextVersion, assetsDir string) ([]string, error) {
	ldflags := fmt.Sprintf("-s -w -X github.com/wakuwaku3/flame/%s/internal/root.Version=%s", module, nextVersion)
	buildRoot := filepath.Join(filepath.Dir(assetsDir), "build-cross")
	if err := os.MkdirAll(buildRoot, fsperm.Dir); err != nil {
		return nil, ex.Wrap(err)
	}
	archives := make([]string, 0, len(targetPlatforms))
	for _, p := range targetPlatforms {
		binaryName := appName
		ext := "tar.gz"
		if p.os == "windows" {
			binaryName = appName + ".exe"
			ext = "zip"
		}
		buildDir := filepath.Join(buildRoot, p.os+"_"+p.arch)
		if err := os.MkdirAll(buildDir, fsperm.Dir); err != nil {
			return nil, ex.Wrap(err)
		}
		fmt.Fprintf(stdout, "build %s %s for %s/%s\n", appName, nextVersion, p.os, p.arch)
		binPath := filepath.Join(buildDir, binaryName)
		const buildEnvCount = 4 // GOOS / GOARCH / CGO_ENABLED / FLAME_LDFLAGS
		env := make([]string, 0, buildEnvCount)
		env = append(env, "GOOS="+p.os, "GOARCH="+p.arch, "CGO_ENABLED=0", "FLAME_LDFLAGS="+ldflags)
		if buildErr := sys.buildBinary(ctx, moduleDir, appDir, binPath, env); buildErr != nil {
			return nil, ex.Wrap(buildErr)
		}
		archive := filepath.Join(assetsDir, fmt.Sprintf("%s_%s_%s_%s.%s", appName, nextVersion, p.os, p.arch, ext))
		if archiveErr := makeArchive(ext, buildDir, binaryName, archive); archiveErr != nil {
			return nil, ex.Wrap(archiveErr)
		}
		archives = append(archives, archive)
	}
	checksumPath := filepath.Join(assetsDir, "SHA256SUMS")
	if err := writeChecksums(archives, checksumPath); err != nil {
		fmt.Fprintf(stderr, "warn: failed to write SHA256SUMS: %s\n", err)
	} else {
		archives = append(archives, checksumPath)
	}
	return archives, nil
}

func makeArchive(ext, srcDir, binaryName, outPath string) error {
	switch ext {
	case "tar.gz":
		return makeTarGz(srcDir, binaryName, outPath)
	case "zip":
		return makeZip(srcDir, binaryName, outPath)
	default:
		return ex.Errorf("unknown archive ext: %s", ext)
	}
}

func makeTarGz(srcDir, binaryName, outPath string) error {
	out, err := os.Create(outPath) //nolint:gosec // G304: assetsDir 配下の固定 path。
	if err != nil {
		return ex.Wrap(err)
	}
	defer out.Close() //nolint:errcheck // best-effort。
	gz := gzip.NewWriter(out)
	defer gz.Close() //nolint:errcheck // best-effort。
	tw := tar.NewWriter(gz)
	defer tw.Close() //nolint:errcheck // best-effort。

	srcPath := filepath.Join(srcDir, binaryName)
	info, err := os.Stat(srcPath)
	if err != nil {
		return ex.Wrap(err)
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return ex.Wrap(err)
	}
	hdr.Name = binaryName
	if writeErr := tw.WriteHeader(hdr); writeErr != nil {
		return ex.Wrap(writeErr)
	}
	src, err := os.Open(srcPath) //nolint:gosec // G304: assetsDir 配下の固定 path。
	if err != nil {
		return ex.Wrap(err)
	}
	defer src.Close() //nolint:errcheck // best-effort。
	if _, copyErr := io.Copy(tw, src); copyErr != nil {
		return ex.Wrap(copyErr)
	}
	return nil
}

func makeZip(srcDir, binaryName, outPath string) error {
	out, err := os.Create(outPath) //nolint:gosec // G304: assetsDir 配下の固定 path。
	if err != nil {
		return ex.Wrap(err)
	}
	defer out.Close() //nolint:errcheck // best-effort。
	zw := zip.NewWriter(out)
	defer zw.Close() //nolint:errcheck // best-effort。

	srcPath := filepath.Join(srcDir, binaryName)
	info, err := os.Stat(srcPath)
	if err != nil {
		return ex.Wrap(err)
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return ex.Wrap(err)
	}
	hdr.Name = binaryName
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return ex.Wrap(err)
	}
	src, err := os.Open(srcPath) //nolint:gosec // G304: assetsDir 配下の固定 path。
	if err != nil {
		return ex.Wrap(err)
	}
	defer src.Close() //nolint:errcheck // best-effort。
	if _, copyErr := io.Copy(w, src); copyErr != nil {
		return ex.Wrap(copyErr)
	}
	return nil
}

func writeChecksums(archives []string, outPath string) error {
	sort.Strings(archives)
	out, err := os.Create(outPath) //nolint:gosec // G304: assetsDir 配下の固定 path。
	if err != nil {
		return ex.Wrap(err)
	}
	defer out.Close() //nolint:errcheck // best-effort。
	for _, a := range archives {
		f, openErr := os.Open(a) //nolint:gosec // G304: 直前に書き出したアーカイブ。
		if openErr != nil {
			return ex.Wrap(openErr)
		}
		h := sha256.New()
		if _, copyErr := io.Copy(h, f); copyErr != nil {
			_ = f.Close()
			return ex.Wrap(copyErr)
		}
		_ = f.Close()
		fmt.Fprintf(out, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), filepath.Base(a))
	}
	return nil
}

// emitDryRunPreview の `nextVersion` は version package との import shadow を避けた命名 (gocritic importShadow)。
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
		summary, openErr := os.OpenFile(stepSummary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fsperm.File) //nolint:gosec // G304: GitHub Actions の既知 path。
		if openErr != nil {
			return ex.Wrap(openErr)
		}
		defer summary.Close() //nolint:errcheck // append-only。
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

// flattenToolSpec は CLI spec JSON を「<path> sub <name>」 = "" / 「<path> flag <name>」 = "<type>:<required>" の flat map に変換する (release-app.sh §diff_cli_spec の jq filter と等価)。 spec の root を含む全 node を DFS で辿り、 各 node 配下の subcommand と flag を独立 entry として emit する。
func flattenToolSpec(specJSON []byte) (map[string]string, error) {
	out := make(map[string]string)
	if err := walkToolNode(specJSON, out); err != nil {
		return nil, ex.Wrap(err)
	}
	return out, nil
}

type toolFlag struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

func walkToolNode(nodeJSON []byte, out map[string]string) error {
	var node struct {
		Path        string            `json:"path"`
		Subcommands []json.RawMessage `json:"subcommands"`
		Flags       []toolFlag        `json:"flags"`
	}
	if err := json.Unmarshal(nodeJSON, &node); err != nil {
		return ex.Wrap(err)
	}
	for _, subRaw := range node.Subcommands {
		var sb struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(subRaw, &sb); err != nil {
			return ex.Wrap(err)
		}
		out[node.Path+" sub "+sb.Name] = ""
	}
	for _, fl := range node.Flags {
		out[node.Path+" flag "+fl.Name] = fl.Type + ":" + boolToString(fl.Required)
	}
	for _, subRaw := range node.Subcommands {
		if err := walkToolNode(subRaw, out); err != nil {
			return err
		}
	}
	return nil
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
