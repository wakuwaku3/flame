package devbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

// 数字始まり + version 様文字のみ。 @latest / ^1.0 / ~1.0 / 1.* を弾き、 pre-release / build tag (1.0.0-rc1, 1.0.0+build) は通す。 1.0.x のような letters 埋め込み wildcard は検出できないが、 devbox では非慣用なため false-negative を許容する。
var concreteVersionRe = regexp.MustCompile(`^\d[0-9A-Za-z.+-]*$`)

func New() clix.Subcommand {
	return clix.NewLeaf("devbox", "devbox 環境定義を検査する", Run)
}

func Run(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check devbox <devbox.json|devbox.lock>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	failed := false
	for _, dir := range uniqueDirs(args) {
		if !checkDir(in.Stderr(), dir) {
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// uniqueDirs は同一 dir に対する重複検査を避けるため引数の dir を順序保持で重複排除する (shell 版の seen_dirs 連想配列と同等)。
func uniqueDirs(args []string) []string {
	seen := make(map[string]struct{}, len(args))
	dirs := make([]string, 0, len(args))
	for _, a := range args {
		d := filepath.Dir(a)
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		dirs = append(dirs, d)
	}
	return dirs
}

func checkDir(stderr io.Writer, dir string) bool {
	jsonPath := filepath.Join(dir, "devbox.json")
	lockPath := filepath.Join(dir, "devbox.lock")

	declared, ok := loadDeclared(stderr, jsonPath)
	if !ok {
		return false
	}
	passed := validateDeclaredVersions(stderr, jsonPath, declared)

	locked, ok := loadLocked(stderr, jsonPath, lockPath)
	if !ok {
		return false
	}

	if !diffDeclaredLocked(stderr, jsonPath, lockPath, declared, locked) {
		passed = false
	}
	return passed
}

// devboxJSON は devbox.json の最小構造で、 .packages を array (`["pkg@ver"]`) と object (`{"pkg": "ver"}`) の両方として解釈するため json.RawMessage で受ける (shell 版の jq クエリと同等)。
type devboxJSON struct {
	Packages json.RawMessage `json:"packages"`
}

type devboxLock struct {
	Packages map[string]json.RawMessage `json:"packages"`
}

func loadDeclared(stderr io.Writer, jsonPath string) ([]string, bool) {
	data, readErr := os.ReadFile(jsonPath) //nolint:gosec // G304: flame check devbox が受け取る path は CLI 起動時 argv そのもので、 検査対象として外部入力を読み込むのが endpoint の責務 (= shell の `jq -- "$json"` と同等の挙動を Go で再現するため、 path は意図的に caller 制御下の任意値)。
	if readErr != nil {
		fmt.Fprintf(stderr, "FAIL: %s: not readable\n", jsonPath)
		return nil, false
	}
	var doc devboxJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: failed to parse JSON / extract .packages\n", jsonPath)
		return nil, false
	}
	if len(doc.Packages) == 0 {
		return nil, true
	}
	declared, parseErr := parsePackages(doc.Packages)
	if parseErr != nil {
		fmt.Fprintf(stderr, "FAIL: %s: failed to parse JSON / extract .packages\n", jsonPath)
		return nil, false
	}
	return declared, true
}

// parsePackages は .packages を array と object 両方の表現から `name@version` 列に正規化する。 devbox は両表現を受理するため shell 版 jq の `if (.packages | type) == "array" ...` と同じ分岐を保つ。
func parsePackages(raw json.RawMessage) ([]string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	switch trimmed[0] {
	case '[':
		var arr []string
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, ex.Wrap(err)
		}
		return arr, nil
	case '{':
		var obj map[string]string
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, ex.Wrap(err)
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]string, 0, len(obj))
		for _, k := range keys {
			out = append(out, k+"@"+obj[k])
		}
		return out, nil
	default:
		return nil, ex.Errorf("unsupported .packages type")
	}
}

func validateDeclaredVersions(stderr io.Writer, jsonPath string, declared []string) bool {
	passed := true
	for _, pkg := range declared {
		idx := strings.LastIndex(pkg, "@")
		if idx < 0 {
			fmt.Fprintf(stderr, "FAIL: %s: package '%s' has no '@<version>' (FLM_ENG_0002 requires an explicit version)\n", jsonPath, pkg)
			passed = false
			continue
		}
		version := pkg[idx+1:]
		if !concreteVersionRe.MatchString(version) {
			fmt.Fprintf(stderr, "FAIL: %s: package '%s' uses non-concrete version '%s' (FLM_ENG_0002 forbids floating specs like @latest)\n", jsonPath, pkg, version)
			passed = false
		}
	}
	return passed
}

func loadLocked(stderr io.Writer, jsonPath, lockPath string) ([]string, bool) {
	data, err := os.ReadFile(lockPath) //nolint:gosec // G304: lockPath は jsonPath と同じ dir から派生する caller 制御下の任意値で、 endpoint 責務として読み込む (loadDeclared と同様)。
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: corresponding devbox.lock not found at %s (FLM_ENG_0002 requires the lock to be tracked)\n", jsonPath, lockPath)
		return nil, false
	}
	var doc devboxLock
	if err := json.Unmarshal(data, &doc); err != nil {
		fmt.Fprintf(stderr, "FAIL: %s: failed to parse JSON / extract .packages keys\n", lockPath)
		return nil, false
	}
	locked := make([]string, 0, len(doc.Packages))
	for k := range doc.Packages {
		// Lock keys は flake ref ("github:NixOS/nixpkgs/...") も含むため、 `name@version` キー (@ 含み : 不含) のみを package 対応分として抽出する。
		if strings.Contains(k, "@") && !strings.Contains(k, ":") {
			locked = append(locked, k)
		}
	}
	return locked, true
}

func diffDeclaredLocked(stderr io.Writer, jsonPath, lockPath string, declared, locked []string) bool {
	declaredSet := uniqueSorted(declared)
	lockedSet := uniqueSorted(locked)
	declaredIndex := toSet(declaredSet)
	lockedIndex := toSet(lockedSet)

	passed := true
	for _, p := range declaredSet {
		if _, ok := lockedIndex[p]; !ok {
			fmt.Fprintf(stderr, "FAIL: %s: package '%s' is declared but missing from %s (run 'devbox install' to update the lock)\n", jsonPath, p, lockPath)
			passed = false
		}
	}
	for _, p := range lockedSet {
		if _, ok := declaredIndex[p]; !ok {
			fmt.Fprintf(stderr, "FAIL: %s: stale entry '%s' is locked but no longer declared in %s (run 'devbox install' to update the lock)\n", lockPath, p, jsonPath)
			passed = false
		}
	}
	return passed
}

func uniqueSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func toSet(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, s := range in {
		out[s] = struct{}{}
	}
	return out
}
