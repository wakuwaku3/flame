package install

import (
	"bytes"
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/ex"
)

// MergeInput は 1 install entry の合成に必要な material。 overlay は無い場合 nil。 fieldalignment 最適化のため、 short-pointer 値 (string 系) を先に置いて []byte (24 bytes) を末尾に並べる (govet fieldalignment hint: GC スキャン対象の pointer bytes を 88 → 80 に縮減)。
type MergeInput struct {
	Strategy       MergeStrategy
	ArrayStrategy  MergeArrayStrategy
	InstallPath    string
	VendorContent  []byte
	OverlayContent []byte
}

// Merge は MergeInput を strategy に従って合成し、 install 先に書き出す byte 列を返す。
func Merge(in *MergeInput) ([]byte, error) {
	switch in.Strategy {
	case MergeReplace:
		if in.OverlayContent != nil {
			return nil, ex.Errorf("merge=replace で overlay は許容されない: %s", in.InstallPath)
		}
		return in.VendorContent, nil
	case MergeAppend:
		if in.OverlayContent == nil {
			return in.VendorContent, nil
		}
		return mergeAppend(in.VendorContent, in.OverlayContent, in.InstallPath), nil
	case MergeDeep:
		if in.OverlayContent == nil {
			return in.VendorContent, nil
		}
		return mergeDeep(in.VendorContent, in.OverlayContent, in.InstallPath, in.ArrayStrategy)
	default:
		return nil, ex.Errorf("unknown merge strategy: %q (path=%s)", in.Strategy, in.InstallPath)
	}
}

func mergeAppend(vendor, overlay []byte, installPath string) []byte {
	if len(vendor) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return vendor
	}
	separator := selectAppendSeparator(installPath, vendor)
	out := make([]byte, 0, len(vendor)+len(separator)+len(overlay))
	out = append(out, vendor...)
	out = append(out, []byte(separator)...)
	out = append(out, overlay...)
	return out
}

// selectAppendSeparator は append 結合の区切りを選ぶ。 markdown は空行 1 行、 それ以外は改行のみ。
func selectAppendSeparator(installPath string, vendor []byte) string {
	ext := strings.ToLower(filepath.Ext(installPath))
	endsWithNewline := bytes.HasSuffix(vendor, []byte("\n"))
	switch ext {
	case ".md":
		if endsWithNewline {
			return "\n"
		}
		return "\n\n"
	default:
		if endsWithNewline {
			return ""
		}
		return "\n"
	}
}

func mergeDeep(vendor, overlay []byte, installPath string, arr MergeArrayStrategy) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(installPath))
	if ext == ".json" {
		return mergeDeepJSON(vendor, overlay, arr)
	}
	return mergeDeepYAML(vendor, overlay, arr)
}

func mergeDeepYAML(vendor, overlay []byte, arr MergeArrayStrategy) ([]byte, error) {
	var v, o yaml.Node
	if err := yaml.Unmarshal(vendor, &v); err != nil {
		return nil, ex.Wrap(err)
	}
	if err := yaml.Unmarshal(overlay, &o); err != nil {
		return nil, ex.Wrap(err)
	}
	merged, err := mergeYAMLNodes(&v, &o, arr)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(merged); err != nil {
		return nil, ex.Wrap(err)
	}
	if err := enc.Close(); err != nil {
		return nil, ex.Wrap(err)
	}
	return buf.Bytes(), nil
}

func mergeYAMLNodes(vendor, overlay *yaml.Node, arr MergeArrayStrategy) (*yaml.Node, error) {
	if vendor.Kind == yaml.DocumentNode && overlay.Kind == yaml.DocumentNode {
		merged, err := mergeYAMLNodes(vendor.Content[0], overlay.Content[0], arr)
		if err != nil {
			return nil, err
		}
		out := *vendor
		out.Content = []*yaml.Node{merged}
		return &out, nil
	}
	if vendor.Kind != overlay.Kind {
		return overlay, nil
	}
	switch vendor.Kind {
	case yaml.MappingNode:
		return mergeYAMLMapping(vendor, overlay, arr)
	case yaml.SequenceNode:
		return mergeYAMLSequence(vendor, overlay, arr), nil
	case yaml.DocumentNode, yaml.ScalarNode, yaml.AliasNode:
		return overlay, nil
	default:
		return overlay, nil
	}
}

func mergeYAMLMapping(vendor, overlay *yaml.Node, arr MergeArrayStrategy) (*yaml.Node, error) {
	out := *vendor
	out.Content = nil
	keys := map[string]int{}
	for i := 0; i+1 < len(vendor.Content); i += keyValueStep {
		k := vendor.Content[i].Value
		keys[k] = len(out.Content)
		out.Content = append(out.Content, vendor.Content[i], vendor.Content[i+1])
	}
	for i := 0; i+1 < len(overlay.Content); i += keyValueStep {
		k := overlay.Content[i]
		v := overlay.Content[i+1]
		if idx, ok := keys[k.Value]; ok {
			merged, err := mergeYAMLNodes(out.Content[idx+1], v, arr)
			if err != nil {
				return nil, err
			}
			out.Content[idx+1] = merged
			continue
		}
		keys[k.Value] = len(out.Content)
		out.Content = append(out.Content, k, v)
	}
	return &out, nil
}

const keyValueStep = 2

func mergeYAMLSequence(vendor, overlay *yaml.Node, arr MergeArrayStrategy) *yaml.Node {
	strategy := arr
	if strategy == "" {
		strategy = MergeArrayAppend
	}
	out := *vendor
	switch strategy {
	case MergeArrayReplace:
		out.Content = append([]*yaml.Node(nil), overlay.Content...)
	case MergeArrayUnique:
		seen := map[string]struct{}{}
		out.Content = nil
		merged := append(append([]*yaml.Node(nil), vendor.Content...), overlay.Content...)
		for _, n := range merged {
			k := yamlKey(n)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out.Content = append(out.Content, n)
		}
	case MergeArrayAppend:
		out.Content = append(append([]*yaml.Node(nil), vendor.Content...), overlay.Content...)
	default:
		out.Content = append(append([]*yaml.Node(nil), vendor.Content...), overlay.Content...)
	}
	return &out
}

func yamlKey(n *yaml.Node) string {
	if n.Kind == yaml.ScalarNode {
		return "s:" + n.Value
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	_ = enc.Encode(n)
	_ = enc.Close()
	return "n:" + buf.String()
}

func mergeDeepJSON(vendor, overlay []byte, arr MergeArrayStrategy) ([]byte, error) {
	var v, o any
	if err := json.Unmarshal(vendor, &v); err != nil {
		return nil, ex.Wrap(err)
	}
	if err := json.Unmarshal(overlay, &o); err != nil {
		return nil, ex.Wrap(err)
	}
	merged := mergeJSONValue(v, o, arr)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(merged); err != nil {
		return nil, ex.Wrap(err)
	}
	return buf.Bytes(), nil
}

func mergeJSONValue(vendor, overlay any, arr MergeArrayStrategy) any {
	vMap, vOK := vendor.(map[string]any)
	oMap, oOK := overlay.(map[string]any)
	if vOK && oOK {
		out := map[string]any{}
		maps.Copy(out, vMap)
		for k, v := range oMap {
			if existing, ok := out[k]; ok {
				out[k] = mergeJSONValue(existing, v, arr)
				continue
			}
			out[k] = v
		}
		return out
	}
	vArr, vAOK := vendor.([]any)
	oArr, oAOK := overlay.([]any)
	if vAOK && oAOK {
		strategy := arr
		if strategy == "" {
			strategy = MergeArrayAppend
		}
		switch strategy {
		case MergeArrayReplace:
			return oArr
		case MergeArrayUnique:
			seen := map[string]struct{}{}
			out := []any{}
			for _, x := range append(vArr, oArr...) {
				key := jsonKey(x)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, x)
			}
			return out
		case MergeArrayAppend:
			return append(append([]any{}, vArr...), oArr...)
		default:
			return append(append([]any{}, vArr...), oArr...)
		}
	}
	return overlay
}

func jsonKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
