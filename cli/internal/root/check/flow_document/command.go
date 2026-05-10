package flow_document

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
	// flowNameParts は ディレクトリ名 <date>__<type>__<title> の "__" 分割数 (FLM_APP_0006)。
	flowNameParts = 3
	monthMin      = 1
	monthMax      = 12
	dayMin        = 1
	dayMax        = 31
	hourMin       = 0
	hourMax       = 23
	minuteMin     = 0
	minuteMax     = 59
)

// 元 shell の date_re / title_re / type 列挙と完全一致させ、 互換挙動を維持する (FLM_APP_0006)。
var (
	dateRePattern  = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})$`)
	titleRePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	allowedTypes   = map[string]struct{}{"spec": {}, "tips": {}, "report": {}}
)

const docsNotesPrefix = "docs/notes/"

func New() clix.Subcommand {
	return clix.NewLeaf("flow-document", "flow ドキュメントのディレクトリ命名を検査する", Run)
}

func Run(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check flow-document <file_under_docs_notes>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ex.Wrap(err)
	}
	flowDirs, failed := collectFlowDirs(args, cwd, in.Stderr())
	for _, flowDir := range flowDirs {
		if !validateFlowDir(cwd, flowDir, in.Stderr()) {
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// collectFlowDirs は入力 path 群を repo-root-relative に正規化し、 docs/notes/<flow_dir> 単位に重複排除して返す。
// 入力は absolute / "./" prefix / repo-root-relative のいずれもあり得るため、
// detect.sh の `(^|/)docs/notes/...` regex と整合させるために repo-root-relative
// に揃えてから判定する (元 shell の rel="${file#./}" / "${rel#"$repo_root"/}" と同等)。
func collectFlowDirs(args []string, cwd string, stderr io.Writer) ([]string, bool) {
	failed := false
	seen := map[string]struct{}{}
	var flowDirs []string
	for _, file := range args {
		rel := strings.TrimPrefix(file, "./")
		rel = strings.TrimPrefix(rel, cwd+string(filepath.Separator))
		if !strings.HasPrefix(rel, docsNotesPrefix) {
			fmt.Fprintf(stderr, "FAIL: %s: not under docs/notes/ (FLM_APP_0006)\n", file)
			failed = true
			continue
		}
		remainder := strings.TrimPrefix(rel, docsNotesPrefix)
		// docs/notes/ 直下にファイルだけを置く (= remainder に "/" が無い) ケースは規約違反。
		// 元 shell の `flow_name=${remainder%%/*}` で remainder と一致するか / 空文字かで判定するのと同等。
		slashIdx := strings.Index(remainder, "/")
		if slashIdx <= 0 {
			fmt.Fprintf(stderr, "FAIL: %s: files directly under docs/notes/ are not allowed; create docs/notes/<dir>/ first (FLM_APP_0006)\n", file)
			failed = true
			continue
		}
		flowDir := docsNotesPrefix + remainder[:slashIdx]
		if _, ok := seen[flowDir]; ok {
			continue
		}
		seen[flowDir] = struct{}{}
		flowDirs = append(flowDirs, flowDir)
	}
	return flowDirs, failed
}

// validateFlowDir は <date>__<type>__<title> の split / 各 part の検査 / index.md 存在を統合した検査単位。
// 元 shell の bash parameter expansion による "__" split を strings.SplitN(s, "__", 3) で代替する。
func validateFlowDir(cwd, flowDir string, stderr io.Writer) bool {
	flowName := filepath.Base(flowDir)
	parts := strings.SplitN(flowName, "__", flowNameParts)
	if len(parts) < flowNameParts {
		fmt.Fprintf(stderr, "FAIL: %s: directory name '%s' must use '__' separators between date / type / title (FLM_APP_0006)\n", flowDir, flowName)
		return false
	}
	datePart, typePart, titlePart := parts[0], parts[1], parts[2]
	ok := validateDate(flowDir, datePart, stderr)
	if _, allowed := allowedTypes[typePart]; !allowed {
		fmt.Fprintf(stderr, "FAIL: %s: type '%s' must be one of: spec, tips, report (FLM_APP_0006)\n", flowDir, typePart)
		ok = false
	}
	if !titleRePattern.MatchString(titlePart) {
		fmt.Fprintf(stderr, "FAIL: %s: title '%s' must be lower snake_case starting with a letter (FLM_APP_0006)\n", flowDir, titlePart)
		ok = false
	}
	if _, statErr := os.Stat(filepath.Join(cwd, flowDir, "index.md")); statErr != nil {
		fmt.Fprintf(stderr, "FAIL: %s: missing index.md entry file (FLM_APP_0006)\n", flowDir)
		ok = false
	}
	return ok
}

// validateDate は yyyymmddhhmm 12 桁を分解し、 月 1-12 / 日 1-31 / 時 0-23 / 分 0-59 のレンジを検査する。
// 形式違反時は range 検査を skip して 1 件の FAIL に集約 (元 shell の if-else 構造と互換)。
func validateDate(flowDir, datePart string, stderr io.Writer) bool {
	match := dateRePattern.FindStringSubmatch(datePart)
	if match == nil {
		fmt.Fprintf(stderr, "FAIL: %s: date '%s' must be 'yyyymmddhhmm' (FLM_APP_0006)\n", flowDir, datePart)
		return false
	}
	ok := true
	if !inRange(match[2], monthMin, monthMax) {
		fmt.Fprintf(stderr, "FAIL: %s: month '%s' out of range (FLM_APP_0006)\n", flowDir, match[2])
		ok = false
	}
	if !inRange(match[3], dayMin, dayMax) {
		fmt.Fprintf(stderr, "FAIL: %s: day '%s' out of range (FLM_APP_0006)\n", flowDir, match[3])
		ok = false
	}
	if !inRange(match[4], hourMin, hourMax) {
		fmt.Fprintf(stderr, "FAIL: %s: hour '%s' out of range (FLM_APP_0006)\n", flowDir, match[4])
		ok = false
	}
	if !inRange(match[5], minuteMin, minuteMax) {
		fmt.Fprintf(stderr, "FAIL: %s: minute '%s' out of range (FLM_APP_0006)\n", flowDir, match[5])
		ok = false
	}
	return ok
}

func inRange(s string, lo, hi int) bool {
	n, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	return n >= lo && n <= hi
}
