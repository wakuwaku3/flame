// Package lib は `flame ci release spec lib <module-path>` subcommand 実装。 配布対象 library module の Go 公開 API surface を AST 走査で抽出し、 JSON spec として stdout に emit する (FLM_FEA_0004 §版番号の決定経路 の library 経路)。 release ワークフロー ([FLM_FEA_0004](../../../../../../docs/adr/feature/FLM_FEA_0004__release_policy.md)) が前回 release との diff から bump kind を判定する入力。
package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

func New() clix.Subcommand {
	return clix.NewLeaf(
		"lib <module-path>",
		"Emit Go API surface JSON for a library module",
		runE,
	)
}

type identifier struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
}

type packageSpec struct {
	Path        string       `json:"path"`
	Identifiers []identifier `json:"identifiers"`
}

type librarySpec struct {
	Module   string        `json:"module"`
	Packages []packageSpec `json:"packages"`
}

func runE(_ context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) != 1 {
		return ex.Errorf("expected exactly 1 module-path argument, got %d", len(args))
	}
	return ex.Wrap(EmitTo(args[0], in.Stdout()))
}

// EmitTo は library spec JSON を w に書き出す package public 経路 (release/lib orchestrator が direct call で spec を取り出すため)。 subcommand 経由 (= runE) と同じロジックを share する。
func EmitTo(modulePath string, w io.Writer) error {
	spec, err := buildSpec(modulePath)
	if err != nil {
		return ex.Wrap(err)
	}
	return ex.Wrap(emitSpec(w, spec))
}

func buildSpec(modulePath string) (*librarySpec, error) {
	importPath, err := readModulePath(modulePath)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	pkgs, err := walkPackages(modulePath)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })
	return &librarySpec{Module: importPath, Packages: pkgs}, nil
}

func readModulePath(modulePath string) (string, error) {
	goModPath := filepath.Join(modulePath, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", ex.Wrapf(err, "read go.mod at %s", goModPath)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", ex.Errorf("module directive not found in %s", goModPath)
}

func walkPackages(modulePath string) ([]packageSpec, error) {
	var pkgs []packageSpec
	err := filepath.WalkDir(modulePath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ex.Wrap(walkErr)
		}
		if !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(modulePath, path)
		if relErr != nil {
			return ex.Wrap(relErr)
		}
		base := d.Name()
		if rel != "." && (base == "internal" || base == "vendor" || base == "testdata" || strings.HasPrefix(base, ".")) {
			return filepath.SkipDir
		}
		ids, parseErr := parsePackage(path)
		if parseErr != nil {
			return ex.Wrap(parseErr)
		}
		if len(ids) > 0 {
			normalized := filepath.ToSlash(rel)
			if normalized == "." {
				normalized = ""
			}
			pkgs = append(pkgs, packageSpec{Path: normalized, Identifiers: ids})
		}
		return nil
	})
	if err != nil {
		return nil, ex.Wrap(err)
	}
	return pkgs, nil
}

func parsePackage(dir string) ([]identifier, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	fset := token.NewFileSet()
	var ids []identifier
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return nil, ex.Wrap(parseErr)
		}
		ids = append(ids, collectExports(fset, file)...)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].Kind != ids[j].Kind {
			return ids[i].Kind < ids[j].Kind
		}
		return ids[i].Name < ids[j].Name
	})
	return ids, nil
}

func collectExports(fset *token.FileSet, file *ast.File) []identifier {
	var ids []identifier
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name != nil && d.Name.IsExported() {
				ids = append(ids, formatFuncDecl(fset, d))
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name != nil && s.Name.IsExported() {
						ids = append(ids, formatTypeSpec(fset, s)...)
					}
				case *ast.ValueSpec:
					for i, name := range s.Names {
						if name.IsExported() {
							ids = append(ids, formatValueSpec(fset, s, name, i, d.Tok))
						}
					}
				}
			}
		}
	}
	return ids
}

func formatFuncDecl(fset *token.FileSet, decl *ast.FuncDecl) identifier {
	name := decl.Name.Name
	kind := "func"
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		kind = "method"
		recv := formatNode(fset, decl.Recv.List[0].Type)
		name = "(" + recv + ")." + name
	}
	return identifier{Name: name, Kind: kind, Signature: formatNode(fset, decl.Type)}
}

// FLM_FEA_0004 §版番号 の判定軸 (公開 surface 各要素の純粋追加 = MINOR / 既存要素の rename・型変更 = MAJOR) と identifier 1 件を 1:1 対応させるため、 struct / interface は型本体と exported field / method に分解する (1 単位の追加が 1 identifier の add に、 既存単位の変更が 1 identifier の shape-change に写る形にして diff 経路を要素単位の判定に揃える)。 type alias (`type Foo = X`) は alias 先そのものを指す宣言なので分解対象外として単一 identifier に保つ。
func formatTypeSpec(fset *token.FileSet, spec *ast.TypeSpec) []identifier {
	typeName := spec.Name.Name
	if spec.Assign.IsValid() {
		sig := "= " + formatNode(fset, spec.Type)
		return []identifier{{Name: typeName, Kind: "type", Signature: sig}}
	}
	switch t := spec.Type.(type) {
	case *ast.StructType:
		return formatStructType(fset, typeName, spec, t)
	case *ast.InterfaceType:
		return formatInterfaceType(fset, typeName, spec, t)
	default:
		sig := formatNode(fset, spec.Type)
		if spec.TypeParams != nil {
			sig = formatNode(fset, spec.TypeParams) + " " + sig
		}
		return []identifier{{Name: typeName, Kind: "type", Signature: sig}}
	}
}

func formatStructType(fset *token.FileSet, typeName string, spec *ast.TypeSpec, t *ast.StructType) []identifier {
	headSig := "struct"
	if spec.TypeParams != nil {
		headSig = formatNode(fset, spec.TypeParams) + " " + headSig
	}
	ids := []identifier{{Name: typeName, Kind: "type", Signature: headSig}}
	if t.Fields == nil {
		return ids
	}
	for _, field := range t.Fields.List {
		fieldSig := formatNode(fset, field.Type)
		if field.Tag != nil {
			fieldSig += " " + field.Tag.Value
		}
		if len(field.Names) == 0 {
			name := embeddedFieldName(field.Type)
			if name == "" || !ast.IsExported(name) {
				continue
			}
			ids = append(ids, identifier{
				Name:      typeName + "." + name,
				Kind:      "field",
				Signature: fieldSig,
			})
			continue
		}
		for _, n := range field.Names {
			if !n.IsExported() {
				continue
			}
			ids = append(ids, identifier{
				Name:      typeName + "." + n.Name,
				Kind:      "field",
				Signature: fieldSig,
			})
		}
	}
	return ids
}

func formatInterfaceType(fset *token.FileSet, typeName string, spec *ast.TypeSpec, t *ast.InterfaceType) []identifier {
	headSig := "interface"
	if spec.TypeParams != nil {
		headSig = formatNode(fset, spec.TypeParams) + " " + headSig
	}
	ids := []identifier{{Name: typeName, Kind: "type", Signature: headSig}}
	if t.Methods == nil {
		return ids
	}
	for _, field := range t.Methods.List {
		if len(field.Names) == 0 {
			// 名前無し element のうち generic constraint の type element (例: `int | float64`) は flame 内の lib で現状未使用のため emit しない (型 element を identifier 化すると順序依存または hash 由来の合成名が必要になり、 公開 API として現実装で得られる便益が無いため)。 埋め込み interface (例: `io.Reader`) のみ name 解決可能なので emit する。
			name := embeddedFieldName(field.Type)
			if name == "" || !ast.IsExported(name) {
				continue
			}
			ids = append(ids, identifier{
				Name:      typeName + "." + name,
				Kind:      "interface_embed",
				Signature: formatNode(fset, field.Type),
			})
			continue
		}
		methodSig := formatNode(fset, field.Type)
		for _, n := range field.Names {
			if !n.IsExported() {
				continue
			}
			ids = append(ids, identifier{
				Name:      typeName + "." + n.Name,
				Kind:      "interface_method",
				Signature: methodSig,
			})
		}
	}
	return ids
}

func embeddedFieldName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if e.Sel == nil {
			return ""
		}
		return e.Sel.Name
	case *ast.StarExpr:
		return embeddedFieldName(e.X)
	case *ast.IndexExpr:
		return embeddedFieldName(e.X)
	case *ast.IndexListExpr:
		return embeddedFieldName(e.X)
	default:
		return ""
	}
}

func formatValueSpec(fset *token.FileSet, spec *ast.ValueSpec, name *ast.Ident, idx int, tok token.Token) identifier {
	kind := "var"
	if tok == token.CONST {
		kind = "const"
	}
	var typeStr string
	if spec.Type != nil {
		typeStr = formatNode(fset, spec.Type)
	}
	var valueStr string
	if idx < len(spec.Values) {
		valueStr = formatNode(fset, spec.Values[idx])
	}
	var parts []string
	if typeStr != "" {
		parts = append(parts, typeStr)
	}
	if valueStr != "" {
		parts = append(parts, "=", valueStr)
	}
	return identifier{Name: name.Name, Kind: kind, Signature: strings.Join(parts, " ")}
}

func formatNode(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, n); err != nil {
		return ""
	}
	return normalizeSpaces(buf.String())
}

// normalizeSpaces は printer 出力に含まれる改行 / tab / 連続空白を 1 つの空白へ畳む。 JSON 内の signature 文字列が複数行になると diff 経路が tab / 行末空白の付加で fragile になるため。
func normalizeSpaces(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
			continue
		}
		b.WriteRune(r)
		inSpace = false
	}
	return strings.TrimSpace(b.String())
}

func emitSpec(w io.Writer, spec *librarySpec) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return ex.Wrap(enc.Encode(spec))
}
