package document

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

// usage 違反 (FLAME_CHECKER_MODE 不正値) は exit 2、 検査失敗は exit 1。
const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

const (
	envCheckerMode     = "FLAME_CHECKER_MODE"
	checkerModeFix     = "fix"
	checkerModeDiag    = "diagnose"
	defaultCheckerMode = checkerModeDiag
)

func New() clix.Subcommand {
	return clix.NewLeaf("document", "Markdown lint と intra-repo link を検査する", Run)
}

func Run(ctx context.Context, in clix.RunInput) error {
	mode, err := resolveMode(os.Getenv(envCheckerMode))
	if err != nil {
		fmt.Fprintln(in.Stderr(), err.Error())
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}

	targets, err := resolveTargets(ctx, in.Args())
	if err != nil {
		return ex.Wrap(err)
	}
	if len(targets) == 0 {
		fmt.Fprintln(in.Stdout(), "no Markdown files to lint")
		return nil
	}

	failed := false
	// markdownlint-cli2 の non-zero 終了は対象 md の lint 違反検出を意味し、 違反内容は cli2 自身が stdout/stderr に既に書いている。 flame 側は exit code のみ failure に倒す。
	if lintErr := runMarkdownlint(ctx, mode, targets, in.Stdout(), in.Stderr()); lintErr != nil {
		failed = true
	}

	if checkLinks(targets, in.Stderr()) {
		failed = true
	}

	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

func resolveMode(raw string) (string, error) {
	mode := raw
	if mode == "" {
		mode = defaultCheckerMode
	}
	if mode != checkerModeFix && mode != checkerModeDiag {
		return "", ex.Errorf("error: invalid %s='%s' (expected 'fix' or 'diagnose')", envCheckerMode, raw)
	}
	return mode, nil
}

func resolveTargets(ctx context.Context, args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	cmd := exec.CommandContext(ctx, "git", "ls-files", "*.md")
	out, err := cmd.Output()
	if err != nil {
		return nil, ex.Wrapf(err, "git ls-files failed")
	}
	var targets []string
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		targets = append(targets, line)
	}
	return targets, nil
}

func runMarkdownlint(ctx context.Context, mode string, targets []string, stdout, stderr io.Writer) error {
	// markdownlint-cli2 不在時は exec が *exec.Error を返すが、 そのまま伝搬すると検査経路の前段 (= devbox 環境の不備) が原因 file 由来の lint 違反と区別できない。 endpoint の責務は「md の検査結果」 を返すことなので、 ここで明示的に診断行を stderr に書いてから failure を返す (shell 版 `set -e` で command not found により即終了する経路と意味的に同期)。
	if _, err := exec.LookPath(markdownlintBinary); err != nil {
		fmt.Fprintf(stderr, "FAIL: %s not found in PATH\n", markdownlintBinary)
		return ex.Wrap(err)
	}
	args := make([]string, 0, len(targets)+1)
	if mode == checkerModeFix {
		args = append(args, "--fix")
	}
	args = append(args, targets...)
	// 起動コマンドは固定文字列 markdownlintBinary、 args の targets は flame check document が受け取る argv そのもので、 検査対象として外部入力 path を cli2 に渡すのが endpoint の責務 (= shell 版 `markdownlint-cli2 "${targets[@]}"` と同等)。 gosec G204 は変数引数の検出のみで意図性まで判定できないため、 当該行に限り false positive として局所抑制する (FLM_GEN_0006)。
	cmd := exec.CommandContext(ctx, markdownlintBinary, args...) //nolint:gosec // G204: flame CLI が受け取った argv を検査対象 path として cli2 に渡す endpoint 責務
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return ex.Wrap(cmd.Run())
}

const markdownlintBinary = "markdownlint-cli2"

// 行頭が ``` で開始するコードフェンスの内側は link 検査対象外 (shell 版 awk 経由の挙動を再現)。
var fenceLine = regexp.MustCompile("^```")

// inline code (`...`) 内の link は検査対象外 (shell 版 gsub `[^`]* ` の挙動を再現)。
var inlineCode = regexp.MustCompile("`[^`]*`")

// markdown link `[text](target)` の target を抽出する。 target には空白を含まない (shell 版 grep `[^) ]+` と同じ抽出規則)。
var linkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^) ]+)\)`)

// URL スキーム ([a-zA-Z][a-zA-Z0-9+.-]*:) を持つ link は intra-repo 対象外 (http: / mailto: 等を除外)。
var urlScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

func checkLinks(files []string, stderr io.Writer) bool {
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: getwd failed: %s\n", err)
		return true
	}
	failed := false
	for _, file := range files {
		info, statErr := os.Stat(file)
		if statErr != nil || info.IsDir() {
			continue
		}
		if checkLinksInFile(file, repoRoot, stderr) {
			failed = true
		}
	}
	return failed
}

func checkLinksInFile(file, repoRoot string, stderr io.Writer) bool {
	data, err := os.ReadFile(file) //nolint:gosec // G304: endpoint 責務は CLI 起動時 argv または git ls-files で得た repo 内 path の md を読み込んで検査結果を返すこと。 caller 制御下の任意 path を読み込むのが意図的な設計 (shell 版の awk 入力と同等)。
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: open failed: %s\n", file, err)
		return true
	}
	fileDir := filepath.Dir(file)
	failed := false
	inFence := false
	for line := range strings.SplitSeq(string(data), "\n") {
		if fenceLine.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		stripped := inlineCode.ReplaceAllString(line, "")
		for _, match := range linkPattern.FindAllStringSubmatch(stripped, -1) {
			link := match[1]
			if !isIntraRepoLink(link) {
				continue
			}
			if !linkResolves(link, fileDir, repoRoot) {
				fmt.Fprintf(stderr, "FAIL: %s: broken link: %s\n", file, link)
				failed = true
			}
		}
	}
	return failed
}

func isIntraRepoLink(link string) bool {
	if link == "" {
		return false
	}
	if urlScheme.MatchString(link) {
		return false
	}
	if strings.HasPrefix(link, "//") {
		return false
	}
	if strings.HasPrefix(link, "#") {
		return false
	}
	return true
}

func linkResolves(link, fileDir, repoRoot string) bool {
	target := link
	if i := strings.Index(target, "#"); i >= 0 {
		target = target[:i]
	}
	if i := strings.Index(target, "?"); i >= 0 {
		target = target[:i]
	}
	if target == "" {
		return true
	}

	var resolved string
	if strings.HasPrefix(target, "/") {
		resolved = filepath.Join(repoRoot, target)
	} else {
		resolved = filepath.Join(fileDir, target)
	}
	_, err := os.Stat(resolved)
	return err == nil
}
