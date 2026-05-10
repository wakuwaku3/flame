package adr

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

// adrRootSegment は ADR 検査の起点となる相対 path 断片。 入力 ADR ファイルから祖先 dir を逆走査してこの断片を見つけ、 当該位置を ADR root として template / numbering 検査の起点に使う。 flame self では downstream ADR を `vendor/flame/docs/adr/` に置き、 internal ADR を `docs/adr/` に置くため、 1 入力集合で複数 root が現れる前提で root を入力 path から動的に決定する (FLM_GEN_0007)。
const adrRootSegment = "docs/adr"

var requiredSections = []string{"## 背景", "## 決定", "## 影響", "## 評価"}

// filename regex は shell 版 `^([A-Z]{3})_([A-Z]{3})_([0-9]{4})__([a-z][a-z0-9_]*)\.md$` と完全一致させる。 group 1 = prefix / group 2 = category / group 3 = number / group 4 = title。
var filenameRe = regexp.MustCompile(`^([A-Z]{3})_([A-Z]{3})_(\d{4})__([a-z][a-z0-9_]*)\.md$`)

// numberRe は連番検査用に filename から number 部分のみ抜き出す。 shell 版 `^[A-Z]{3}_[A-Z]{3}_([0-9]{4})__` と一致。
var numberRe = regexp.MustCompile(`^[A-Z]{3}_[A-Z]{3}_(\d{4})__`)

// categoryDirs は category id → 期待する category directory 名の対応表。 shell 版 `category_dirs` と同集合 / 同 mapping。
var categoryDirs = map[string]string{
	"GEN": "general",
	"APP": "application",
	"ENG": "engineering",
	"FEA": "feature",
	"INF": "infrastructure",
	"SPC": "specific",
}

func New() clix.Subcommand {
	return clix.NewLeaf("adr", "ADR ファイルを静的検査する", Run)
}

func Run(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) == 0 {
		fmt.Fprintln(in.Stderr(), "usage: flame check adr <adr_file>...")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	failed := false
	for _, file := range args {
		if !checkFile(in.Stderr(), file) {
			failed = true
		}
	}
	for _, root := range adrRootsForFiles(args) {
		if !checkRepository(in.Stderr(), root) {
			failed = true
		}
	}
	if failed {
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	return nil
}

// checkFile は 1 ADR ファイルに対する 4 観点 (存在 / filename / category 配置 / 必須セクション) を検査し、 全 pass で true を返す。 shell 版の per-file ループと同じ順序・同じ FAIL message を維持する。
func checkFile(stderr io.Writer, file string) bool {
	rel := relativePath(file)
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		failLine(stderr, "%s: file does not exist", rel)
		return false
	}

	base := filepath.Base(file)
	parent := filepath.Base(filepath.Dir(file))

	m := filenameRe.FindStringSubmatch(base)
	if m == nil {
		failLine(stderr, "%s: filename does not match {PREFIX}_{CATEGORY}_{NUMBER}__{snake_case_title}.md", rel)
		return false
	}
	category := m[2]

	ok := true
	expectedDir, known := categoryDirs[category]
	if !known {
		failLine(stderr, "%s: unknown category id '%s' (allowed: %s)", rel, category, allowedCategoriesText())
		ok = false
	} else if parent != expectedDir {
		failLine(stderr, "%s: category '%s' should live in 'docs/adr/%s/' (found in 'docs/adr/%s/')", rel, category, expectedDir, parent)
		ok = false
	}

	for _, section := range requiredSections {
		if !fileContainsLiteral(file, section) {
			failLine(stderr, "%s: missing required section: %s", rel, section)
			ok = false
		}
	}
	return ok
}

// adrRootsForFiles は入力 ADR ファイル list から template / numbering 検査の対象となる ADR root path 集合を導出する。 各入力 path の祖先 dir のうち、 `docs/adr` で終わるものを root として採用し、 sort + dedup して順序を決定論化する。 入力が空 / 該当 dir なし の場合は cwd 直下 `docs/adr` を fallback として返し (= 既存の挙動と一致する経路)、 root が見つからない呼び出しでも従来同様の missing template 失敗が再現できるようにする。
func adrRootsForFiles(files []string) []string {
	seen := make(map[string]struct{})
	roots := make([]string, 0)
	for _, f := range files {
		root := findADRRootFor(f)
		if root == "" {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	if len(roots) == 0 {
		return []string{adrRootSegment}
	}
	sort.Strings(roots)
	return roots
}

// findADRRootFor は file path の祖先 dir 列を上に辿り、 末尾が `docs/adr` で終わる最初の dir を ADR root として返す。 typical 入力は `vendor/flame/docs/adr/<category>/<file>.md` または `docs/adr/<category>/<file>.md` で、 前者は `vendor/flame/docs/adr` を返す。 該当 dir が無い場合は空文字を返し、 caller 側の fallback (cwd 相対 `docs/adr`) に帰着させる。
func findADRRootFor(file string) string {
	dir := filepath.Dir(file)
	for {
		if strings.HasSuffix(filepath.ToSlash(dir), "/"+adrRootSegment) || filepath.ToSlash(dir) == adrRootSegment {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// checkRepository は ADR root 単位で 2 観点 (template 存在 / 各カテゴリ dir の dense 連番) を検査する。 shell 版の `find "$category_dir" -maxdepth 1 -name '*.md'` と同じく非再帰で .md のみを対象にする。
func checkRepository(stderr io.Writer, root string) bool {
	ok := true
	templatePath := filepath.Join(root, "adr_template.md")
	if !fileExists(templatePath) {
		failLine(stderr, "missing template: %s", relativePath(templatePath))
		ok = false
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return ok
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		categoryDir := filepath.Join(root, entry.Name())
		if !checkCategoryNumbering(stderr, categoryDir) {
			ok = false
		}
	}
	return ok
}

// checkCategoryNumbering は 1 category directory 内の ADR 番号が `0001` から dense 連番であることを検査する。 重複検出時はそこで FAIL を出して以降の連番チェックは継続する (shell 版は重複と連番不一致を別 if で扱うため、 同じ処理順を維持)。
func checkCategoryNumbering(stderr io.Writer, categoryDir string) bool {
	files, err := os.ReadDir(categoryDir)
	if err != nil {
		return true
	}

	numbers := make([]string, 0)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) != ".md" {
			continue
		}
		m := numberRe.FindStringSubmatch(f.Name())
		if m == nil {
			continue
		}
		numbers = append(numbers, m[1])
	}
	if len(numbers) == 0 {
		return true
	}

	sorted := uniqueSorted(numbers)
	rel := relativePath(categoryDir) + "/"
	ok := true
	if len(sorted) != len(numbers) {
		failLine(stderr, "%s: duplicate ADR numbers detected", rel)
		ok = false
	}
	expected := 1
	for _, n := range sorted {
		actual, convErr := strconv.Atoi(n)
		if convErr != nil {
			continue
		}
		if actual != expected {
			failLine(stderr, "%s: numbering must be dense from 0001 (expected %04d, got %s)", rel, expected, n)
			ok = false
			break
		}
		expected++
	}
	return ok
}

// fileContainsLiteral は shell 版 `grep -qF` と同じ literal 部分一致を行う。 行頭限定にすると `## 背景 (補足)` 等を弾けないので shell 版と挙動を揃えるため部分一致のままにする。
func fileContainsLiteral(path, needle string) bool {
	data, err := os.ReadFile(path) //nolint:gosec // G304: flame check adr が受け取る path は CLI 起動時 argv そのもので、 検査対象として外部入力を読み込むのが endpoint の責務 (= shell の `grep -qF "$section" "$file"` と同等の挙動を Go で再現するため、 path は意図的に caller 制御下の任意値)。
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// uniqueSorted は重複除去 + 昇順 sort 後の slice を返す。 shell 版 `sort -u` と同じ意味で、 連番検査の入力に使う。
func uniqueSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// allowedCategoriesText は unknown category id 失敗時の許容集合 message。 shell 版 `${!category_dirs[*]}` は連想配列キーの順序未定義のため、 Go 側は決定論的に sort 済 list を返す (test の安定化目的)。
func allowedCategoriesText() string {
	keys := make([]string, 0, len(categoryDirs))
	for k := range categoryDirs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, " ")
}

// relativePath は cwd (= repo root 前提) からの相対 path を返す。 shell 版 `${file#"$repo_root"/}` と同じ役割。 path traversal 等で repo root 外を指す入力でも shell 版は元 path を絶対のまま出力するため、 Go 側も filepath.Rel が失敗した場合は元 path を返して挙動を揃える。
func relativePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}

func failLine(stderr io.Writer, format string, args ...any) {
	fmt.Fprintf(stderr, "FAIL: "+format+"\n", args...)
}
