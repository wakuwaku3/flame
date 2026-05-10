// Package schemavalidate は flame 独自型の schema 規約 (FLM_APP_0003 / FLM_APP_0004
// §flame 独自型の schema 規約) に従った schema 検査経路を提供する。
//
// 検査対象 file `<path>/<basename>` に対して、 `vendor/flame/schemas/<basename>.schema.<ext>`
// を search-upward で探し、 当該 schema が存在する file は flame 独自型と判定する。
package schemavalidate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/ex"
)

// FindFlameSchema は filePath の親方向に `vendor/flame/schemas/<basename>.schema.<ext>`
// を探し、 見つかった絶対 path を返す。 ext は "yaml" / "json" のいずれか
// (FLM_APP_0003 / FLM_APP_0004 で「schema 自体のシリアライズ形式は対象 file と同言語」
// と規定されているため)。
//
// 戻り値は (path, found, error) の 3 値。 schema 不在は (false, nil) で表し、
// 不在以外の stat error (permission denied 等) は (false, error) で上位に伝える
// (= 不在と stat 失敗を呼び出し側で区別できるようにする。 silent な握りつぶしで
// schema 検査が事実上 skip される事故を防ぐ)。
func FindFlameSchema(filePath, ext string) (path string, found bool, err error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return "", false, ex.Wrapf(err, "resolve abs path")
	}
	base := filepath.Base(abs)
	dir := filepath.Dir(abs)
	for {
		cand := filepath.Join(dir, "vendor", "flame", "schemas", base+".schema."+ext)
		info, statErr := os.Stat(cand)
		if statErr == nil && !info.IsDir() {
			return cand, true, nil
		}
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return "", false, ex.Wrapf(statErr, "stat schema candidate")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func ResolveDirectivePath(filePath, directive string) (string, error) {
	if filepath.IsAbs(directive) {
		return filepath.Clean(directive), nil
	}
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return "", ex.Wrapf(err, "resolve abs path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(absFile), directive)), nil
}

// CompileSchema は JSON Schema 仕様の schema (YAML or JSON シリアライズ) を
// schemaPath から読んで compile する。 yaml.v3 が unmarshal 結果の map のキーに
// `any` 型を返す可能性に備え Normalize で string キーへ正規化する。
func CompileSchema(schemaPath string) (*jsonschema.Schema, error) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, ex.Wrapf(err, "read schema")
	}
	var doc any
	if uerr := yaml.Unmarshal(data, &doc); uerr != nil {
		return nil, ex.Wrapf(uerr, "parse schema")
	}
	doc = Normalize(doc)
	c := jsonschema.NewCompiler()
	url := "file:///" + filepath.ToSlash(filepath.Clean(schemaPath))
	if aerr := c.AddResource(url, doc); aerr != nil {
		return nil, ex.Wrapf(aerr, "add schema")
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, ex.Wrapf(err, "compile schema")
	}
	return sch, nil
}

// Normalize は yaml.v3 unmarshal 結果に含まれる可能性のある map[any]any を
// map[string]any へ再帰的に変換する。 jsonschema/v6 は map[string]any / []any /
// scalar 値を期待するため、 mixed-key map が含まれていると validation 段で
// 型エラーになる経路を未然に塞ぐ。
func Normalize(v any) any {
	switch x := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[fmt.Sprint(k)] = Normalize(val)
		}
		return m
	case map[string]any:
		for k, val := range x {
			x[k] = Normalize(val)
		}
		return x
	case []any:
		for i, val := range x {
			x[i] = Normalize(val)
		}
		return x
	}
	return v
}
