// Package bucketize は file 群を content type 別の checker に振り分ける classifier (FLM_ENG_0003 §検査対象ファイルの決定 / FLM_FEA_0001 の trigger / target 分離)。
package bucketize

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wakuwaku3/flame/lib/ex"
)

// Entry は 1 checker に振り分けられた検査対象。 Checker は `flame check run` が登録 registry の lookup key として用いる識別子で、 dispatch.Dispatch / `flame check run` 経由で対応する flame サブコマンドの Run 関数に解決される。 Targets はファイル path (markdown / shell / json 等) または Go package path (Go 系 checker のとき) で、 各 checker が期待する形式に合わせて caller が組み立てたものをそのまま渡す。 rune 順 sort 済で決定論的。
type Entry struct {
	Checker string
	Targets []string
}

const (
	CheckerDocument      = "document"
	CheckerADR           = "adr"
	CheckerShell         = "shell"
	CheckerJSON          = "json"
	CheckerYAML          = "yaml"
	CheckerDevbox        = "devbox"
	CheckerFlowDocument  = "flow-document"
	CheckerGitHubActions = "github-actions"
	CheckerGoLint        = "go-lint"
	CheckerGoBuild       = "go-build"
	CheckerGoTest        = "go-test"
)

// docs/adr 配下は `<category>/<file>.md` の 2 階層構造、 docs/notes 配下は flow document 配置 (FLM_APP_0001 §flow)、 .github/workflows は yaml / yml 両許容。
var (
	adrPathRe       = regexp.MustCompile(`(^|/)docs/adr/[^/]+/[^/]+\.md$`)
	flowDocPathRe   = regexp.MustCompile(`(^|/)docs/notes/[^/]+/.+`)
	ghaWorkflowRe   = regexp.MustCompile(`(^|/)\.github/workflows/[^/]+\.ya?ml$`)
	mainPackageDecl = regexp.MustCompile(`^package[\t ]+main([\t ]|$)`)
)

// Bucketize は files を content type 別の checker bucket に振り分ける。 repoRoot は Go package enumeration で go.mod 探索 / file 列挙の起点となる絶対 path で、 caller 側で `os.Getwd()` 等から渡す。 戻り値は checker 名で sort 済の決定論的 list。 1 file が複数 bucket に該当することは仕様 (例: devbox.json は json + devbox)。
func Bucketize(repoRoot string, files []string) ([]Entry, error) {
	if len(files) == 0 {
		return nil, nil
	}
	c := newClassifier(repoRoot)
	for _, f := range files {
		c.classifyFile(f)
	}
	if err := c.resolveGoTargets(); err != nil {
		return nil, ex.Wrap(err)
	}
	return c.toEntries(), nil
}

type classifier struct {
	repoRoot          string
	markdownFiles     []string
	adrFiles          []string
	shellFiles        []string
	jsonFiles         []string
	yamlFiles         []string
	devboxFiles       []string
	flowDocFiles      []string
	ghaWorkflowFiles  []string
	goModuleRoots     []string
	seenGoModuleRoots map[string]struct{}
	goLintTargets     []string
	goBuildTargets    []string
	goTestTargets     []string
}

func newClassifier(repoRoot string) *classifier {
	return &classifier{
		repoRoot:          repoRoot,
		markdownFiles:     nil,
		adrFiles:          nil,
		shellFiles:        nil,
		jsonFiles:         nil,
		yamlFiles:         nil,
		devboxFiles:       nil,
		flowDocFiles:      nil,
		ghaWorkflowFiles:  nil,
		goModuleRoots:     nil,
		seenGoModuleRoots: make(map[string]struct{}),
		goLintTargets:     nil,
		goBuildTargets:    nil,
		goTestTargets:     nil,
	}
}

func (c *classifier) classifyFile(f string) {
	if strings.HasSuffix(f, ".md") {
		c.markdownFiles = append(c.markdownFiles, f)
	}
	if adrPathRe.MatchString(f) {
		c.adrFiles = append(c.adrFiles, f)
	}
	if strings.HasSuffix(f, ".sh") {
		c.shellFiles = append(c.shellFiles, f)
	}
	if strings.HasSuffix(f, ".json") {
		c.jsonFiles = append(c.jsonFiles, f)
	}
	if strings.HasSuffix(f, ".yaml") || strings.HasSuffix(f, ".yml") {
		c.yamlFiles = append(c.yamlFiles, f)
	}
	base := filepath.Base(f)
	if base == "devbox.json" || base == "devbox.lock" {
		c.devboxFiles = append(c.devboxFiles, f)
	}
	if flowDocPathRe.MatchString(f) {
		c.flowDocFiles = append(c.flowDocFiles, f)
	}
	if ghaWorkflowRe.MatchString(f) {
		c.ghaWorkflowFiles = append(c.ghaWorkflowFiles, f)
	}
	if isGoFile(f) {
		if moduleRoot, ok := c.resolveGoModuleRoot(f); ok {
			if _, seen := c.seenGoModuleRoots[moduleRoot]; !seen {
				c.seenGoModuleRoots[moduleRoot] = struct{}{}
				c.goModuleRoots = append(c.goModuleRoots, moduleRoot)
			}
		}
	}
}

func isGoFile(f string) bool {
	if strings.HasSuffix(f, ".go") {
		return true
	}
	base := filepath.Base(f)
	return base == "go.mod" || base == "go.sum"
}

// resolveGoModuleRoot は `f` を内包する go.mod 配置ディレクトリを repoRoot 配下で探す。 repoRoot 外 / module 不在は (false) で返して caller 側で検査対象から除外する。 file が dir の場合 (将来拡張) は dir 自身を起点に walk-up する。
func (c *classifier) resolveGoModuleRoot(f string) (string, bool) {
	abs := f
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(c.repoRoot, f)
	}
	info, err := os.Stat(abs)
	var dir string
	if err == nil && info.IsDir() {
		dir = abs
	} else {
		dir = filepath.Dir(abs)
	}
	if !isWithin(dir, c.repoRoot) {
		return "", false
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func isWithin(child, ancestor string) bool {
	rel, err := filepath.Rel(ancestor, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..")
}

// resolveGoTargets は seen go module 配下を再帰列挙し、 各 package を lint target に必ず登録、 main package は build target、 *_test.go を含む package は test target に追加で登録する。
func (c *classifier) resolveGoTargets() error {
	if len(c.goModuleRoots) == 0 {
		return nil
	}
	seenLint := make(map[string]struct{})
	seenBuild := make(map[string]struct{})
	seenTest := make(map[string]struct{})
	for _, moduleRoot := range c.goModuleRoots {
		pkgDirs, err := enumeratePackageDirs(moduleRoot)
		if err != nil {
			return ex.Wrap(err)
		}
		for _, pkgAbs := range pkgDirs {
			pkgRel, relErr := filepath.Rel(c.repoRoot, pkgAbs)
			if relErr != nil {
				return ex.Wrap(relErr)
			}
			if _, ok := seenLint[pkgRel]; !ok {
				seenLint[pkgRel] = struct{}{}
				c.goLintTargets = append(c.goLintTargets, pkgRel)
			}
			if isMainPackageDir(pkgAbs) {
				if _, ok := seenBuild[pkgRel]; !ok {
					seenBuild[pkgRel] = struct{}{}
					c.goBuildTargets = append(c.goBuildTargets, pkgRel)
				}
			}
			if hasTestFilesDir(pkgAbs) {
				if _, ok := seenTest[pkgRel]; !ok {
					seenTest[pkgRel] = struct{}{}
					c.goTestTargets = append(c.goTestTargets, pkgRel)
				}
			}
		}
	}
	return nil
}

// enumeratePackageDirs は moduleRoot 配下の *.go を含む全ディレクトリを sort 済の絶対 path 一覧で返す。 vendor / .git / .devbox / .direnv は走査負荷削減のため prune する。
func enumeratePackageDirs(moduleRoot string) ([]string, error) {
	dirSet := make(map[string]struct{})
	err := filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ex.Wrap(walkErr)
		}
		if d.IsDir() {
			name := d.Name()
			if path != moduleRoot && (name == "vendor" || name == ".git" || name == ".devbox" || name == ".direnv") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") {
			dirSet[filepath.Dir(path)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, ex.Wrap(err)
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// isMainPackageDir は pkgDir 直下の *.go (test 除く) を読んで `package main` 宣言があれば true。 *_test.go の `package main_test` を main 判定から除外するため除く。 ファイル read 失敗は無視 (= main 判定 false 寄り) する。
func isMainPackageDir(pkgDir string) bool {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(pkgDir, name)) //nolint:gosec // G304: pkgDir は repoRoot 配下を WalkDir で得た internal path で外部入力ではない。
		if readErr != nil {
			continue
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			if mainPackageDecl.MatchString(strings.TrimSpace(line)) {
				return true
			}
			if strings.HasPrefix(strings.TrimSpace(line), "package ") {
				break
			}
		}
	}
	return false
}

func hasTestFilesDir(pkgDir string) bool {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			return true
		}
	}
	return false
}

// toEntries は分類済みデータを Entry list に変換し、 Checker 名で安定 sort して emit する。 並列 dispatch の決定論性および test 結果の安定化のため明示 sort する。
func (c *classifier) toEntries() []Entry {
	var entries []Entry
	add := func(checker string, targets []string) {
		if len(targets) == 0 {
			return
		}
		entries = append(entries, Entry{Checker: checker, Targets: targets})
	}
	add(CheckerDocument, c.markdownFiles)
	add(CheckerADR, c.adrFiles)
	add(CheckerShell, c.shellFiles)
	add(CheckerJSON, c.jsonFiles)
	add(CheckerYAML, c.yamlFiles)
	add(CheckerDevbox, c.devboxFiles)
	add(CheckerFlowDocument, c.flowDocFiles)
	add(CheckerGitHubActions, c.ghaWorkflowFiles)
	add(CheckerGoLint, c.goLintTargets)
	add(CheckerGoBuild, c.goBuildTargets)
	add(CheckerGoTest, c.goTestTargets)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Checker < entries[j].Checker })
	return entries
}
