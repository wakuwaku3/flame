// Package detect は `flame ci check detect <github_output_path>` subcommand。 PR base/head の差分を gh api compare → git fetch (merge_base + ahead_by 分 deepen) → git diff で算出し、 さらに bucket 化 (= 内部 bucketize package による content type 振り分け) して GitHub Actions の step output に `files=` / `matrix=` / `has_work=` を emit する。 移行元 shell `.github/scripts/compute-changed-files.sh` + `.github/scripts/bucket-changed-files.sh` + `scripts/detect.sh` を 1 endpoint に統合した Go ネイティブ実装 (FLM_FEA_0004 §責務範囲: CI 補助 / FLM_ENG_0003 §検査対象ファイルの決定)。
package detect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"

	"github.com/wakuwaku3/flame/cli/internal/check/bucketize"
	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

func New() clix.Subcommand {
	return clix.NewLeaf("detect", "PR 差分から files / checker matrix を算出して GITHUB_OUTPUT に emit する", run)
}

func run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) != 1 {
		fmt.Fprintln(in.Stderr(), "usage: flame ci check detect <github_output_path>")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	return doRun(ctx, in.Stdout(), in.Stderr(), args[0], &execOps{})
}

// ops は外部副作用 (gh api / git / scripts/detect.sh) を test 内で fake に差し替える fault boundary。
type ops interface {
	ghCompare(ctx context.Context, repo, baseSHA, headSHA string) ([]byte, error)
	gitFetch(ctx context.Context, args ...string) error
	gitDiffNamesZ(ctx context.Context, mergeBaseDotsHead string) ([]byte, error)
	bucketize(ctx context.Context, files []string) ([]bucketEntry, error)
}

type execOps struct{}

func (*execOps) ghCompare(ctx context.Context, repo, baseSHA, headSHA string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", "api", fmt.Sprintf("repos/%s/compare/%s...%s", repo, baseSHA, headSHA)) //nolint:gosec // G204: gh 起動 + caller 内部値 (repo / SHA は env / workflow input)。
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, ex.Wrapf(err, "gh api compare failed: %s", stderr.String())
	}
	return stdout.Bytes(), nil
}

func (*execOps) gitFetch(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"fetch"}, args...)...) //nolint:gosec // G204: caller (本 endpoint) が固定 binary 名 + 内部組立 argv を渡す経路で外部入力ではない。
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return ex.Wrap(cmd.Run())
}

func (*execOps) gitDiffNamesZ(ctx context.Context, mergeBaseDotsHead string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "-z", "--name-only", "--diff-filter=ACMR", mergeBaseDotsHead) //nolint:gosec // G204: git 起動 + caller 内部値 (mergeBaseDotsHead は本 endpoint で組み立てた SHA)。
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, ex.Wrapf(err, "git diff failed: %s", stderr.String())
	}
	return stdout.Bytes(), nil
}

// bucketize は cli/internal/check/bucketize package を呼び出して file 群を checker bucket に振り分ける。 旧 shell 版 `scripts/detect.sh` の Go ネイティブ移植経路で、 cli から bash subprocess を起動しない (FLM_FEA_0004: cli は cli/scripts/install.sh と .github/workflows/tests/*.sh 以外の shell を呼ばない)。
func (*execOps) bucketize(_ context.Context, files []string) ([]bucketEntry, error) {
	if len(files) == 0 {
		return nil, nil
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, ex.Wrap(err)
	}
	entries, err := bucketize.Bucketize(repoRoot, files)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	out := make([]bucketEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, bucketEntry{Checker: e.Checker, Files: e.Targets})
	}
	return out, nil
}

// bucketEntry は matrix include 1 entry。 shell 版の `{checker, files}` と同じキー名 / 型。
type bucketEntry struct {
	Checker string   `json:"checker"`
	Files   []string `json:"files"`
}

type matrixOut struct {
	Include []bucketEntry `json:"include"`
}

// merge_base SHA は 40 桁 hex string (FLM_ENG_0003 §最小 clone)。 compare API レスポンスの sanity check に使う。
var sha40Re = regexp.MustCompile(`^[0-9a-f]{40}$`)

func doRun(ctx context.Context, stdout, stderr io.Writer, githubOutputPath string, op ops) error {
	baseSHA, err := requireEnv("BASE_SHA")
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	headSHA, err := requireEnv("HEAD_SHA")
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
	fetchRef := os.Getenv("FETCH_REF")

	compareJSON, ghErr := op.ghCompare(ctx, repo, baseSHA, headSHA)
	if ghErr != nil {
		return ex.Wrap(ghErr)
	}
	var compare struct {
		MergeBaseCommit struct {
			SHA string `json:"sha"`
		} `json:"merge_base_commit"`
		AheadBy int `json:"ahead_by"`
	}
	if jsonErr := json.Unmarshal(compareJSON, &compare); jsonErr != nil {
		return ex.Wrapf(jsonErr, "decode compare JSON")
	}
	mergeBase := compare.MergeBaseCommit.SHA
	if !sha40Re.MatchString(mergeBase) {
		fmt.Fprintf(stderr, "ERROR: invalid merge_base SHA from compare API: %s\n", mergeBase)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if compare.AheadBy < 0 {
		fmt.Fprintf(stderr, "ERROR: invalid ahead_by from compare API: %d\n", compare.AheadBy)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}

	if fetchErr := op.gitFetch(ctx, "--no-tags", "--quiet", "--depth=1", "origin", mergeBase); fetchErr != nil {
		return ex.Wrap(fetchErr)
	}
	if compare.AheadBy > 0 {
		args := []string{"--no-tags", "--quiet", fmt.Sprintf("--deepen=%d", compare.AheadBy), "origin"}
		if fetchRef != "" {
			args = append(args, fetchRef)
		}
		if fetchErr := op.gitFetch(ctx, args...); fetchErr != nil {
			return ex.Wrap(fetchErr)
		}
	}

	rawDiff, err := op.gitDiffNamesZ(ctx, mergeBase+"..."+headSHA)
	if err != nil {
		return ex.Wrap(err)
	}
	files := splitNUL(rawDiff)

	filesJSON, err := json.Marshal(filesOrEmpty(files))
	if err != nil {
		return ex.Wrap(err)
	}

	buckets, err := op.bucketize(ctx, files)
	if err != nil {
		return ex.Wrap(err)
	}
	matrix := matrixOut{Include: bucketsOrEmpty(buckets)}
	matrixJSON, err := json.Marshal(matrix)
	if err != nil {
		return ex.Wrap(err)
	}
	hasWork := len(buckets) > 0

	return ex.Wrap(emitOutputs(stdout, githubOutputPath,
		fmt.Sprintf("files=%s", filesJSON),
		fmt.Sprintf("matrix=%s", matrixJSON),
		fmt.Sprintf("has_work=%t", hasWork),
	))
}

func filesOrEmpty(files []string) []string {
	if files == nil {
		return []string{}
	}
	return files
}

func bucketsOrEmpty(buckets []bucketEntry) []bucketEntry {
	if buckets == nil {
		return []bucketEntry{}
	}
	return buckets
}

func splitNUL(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, string(p))
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

func emitOutputs(stdout io.Writer, githubOutput string, lines ...string) error {
	f, err := os.OpenFile(githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fsperm.File) //nolint:gosec // G304: GitHub Actions が GITHUB_OUTPUT に書き出す既知 path への append。
	if err != nil {
		return ex.Wrap(err)
	}
	defer f.Close() //nolint:errcheck // append-only step output。
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
