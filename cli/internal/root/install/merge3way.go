package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/ex"
)

// Merge3WayInput は 1 install entry を 3-way merge で合成する材料 (FLM_FEA_0003 §副ファイル overlay 機構)。 base = 前回 install 時の vendor snapshot (`flame.lock.files[].vendor_content`)、 their = 現状の vendor、 our = 現状の overlay (= 利用者が記述した「最終形」)。 fieldalignment を満たすため short-pointer 値を先、 []byte (24B) を末尾に並べる。
type Merge3WayInput struct {
	Strategy     MergeStrategy
	InstallPath  string
	BaseContent  []byte
	TheirContent []byte
	OurContent   []byte
}

// MergeConflict は 3-way merge で base / their / our の差分が両立しない位置を表す。 path は "." 区切りの YAML / JSON path。 sequence 内の conflict は path 末尾に "[i]" を付与する。
type MergeConflict struct {
	Path        string
	Description string
}

// Merge3WayOutput は 3-way merge の結果。 Content には合成結果 (conflict が無い場合のみ意味を持つ)。 Conflicts が空でなければ install 先は更新せず、 caller で conflict 報告 + overlay marker 書き出しを行う。
type Merge3WayOutput struct {
	Content   []byte
	Conflicts []MergeConflict
}

// Merge3Way は base / their / our の 3 入力から overlay の意味論「最終形」 を反映した合成結果を返す。 our (overlay) が無い場合は their (vendor) をそのまま返す (= overlay 不在時の 0-merge 経路)。 base が無い (= 初回 install with overlay) 場合は our をそのまま返す (= 利用者が完成形を書いた前提)。 strategy=replace は overlay 不可 (vendor のみ)。 strategy=append は line-based 3-way merge を行う。 strategy=deep は YAML / JSON 構造化 3-way を行う。
func Merge3Way(in *Merge3WayInput) (*Merge3WayOutput, error) {
	switch in.Strategy {
	case MergeReplace:
		if in.OurContent != nil {
			return nil, ex.Errorf("merge=replace で overlay は許容されない: %s", in.InstallPath)
		}
		return &Merge3WayOutput{Content: in.TheirContent, Conflicts: nil}, nil
	case MergeAppend:
		return merge3WayText(in)
	case MergeDeep:
		return merge3WayStructured(in)
	default:
		return nil, ex.Errorf("unknown merge strategy: %q (path=%s)", in.Strategy, in.InstallPath)
	}
}

func merge3WayText(in *Merge3WayInput) (*Merge3WayOutput, error) {
	if in.OurContent == nil {
		return &Merge3WayOutput{Content: in.TheirContent, Conflicts: nil}, nil
	}
	if len(in.BaseContent) == 0 {
		// 初回 install with overlay: overlay を完成形として採用 (base が記録されていない場合も同様)
		return &Merge3WayOutput{Content: in.OurContent, Conflicts: nil}, nil
	}
	merged, conflicts := mergeLines3Way(in.BaseContent, in.TheirContent, in.OurContent)
	return &Merge3WayOutput{Content: merged, Conflicts: conflicts}, nil
}

func merge3WayStructured(in *Merge3WayInput) (*Merge3WayOutput, error) {
	if in.OurContent == nil {
		return &Merge3WayOutput{Content: in.TheirContent, Conflicts: nil}, nil
	}
	if len(in.BaseContent) == 0 {
		return &Merge3WayOutput{Content: in.OurContent, Conflicts: nil}, nil
	}
	ext := strings.ToLower(filepath.Ext(in.InstallPath))
	if ext == ".json" {
		return merge3WayJSON(in)
	}
	return merge3WayYAML(in)
}

func merge3WayYAML(in *Merge3WayInput) (*Merge3WayOutput, error) {
	var base, their, our yaml.Node
	if err := yaml.Unmarshal(in.BaseContent, &base); err != nil {
		return nil, ex.Wrapf(err, "parse base yaml: %s", in.InstallPath)
	}
	if err := yaml.Unmarshal(in.TheirContent, &their); err != nil {
		return nil, ex.Wrapf(err, "parse their yaml: %s", in.InstallPath)
	}
	if err := yaml.Unmarshal(in.OurContent, &our); err != nil {
		return nil, ex.Wrapf(err, "parse our yaml: %s", in.InstallPath)
	}
	merged, conflicts := merge3WayYAMLNodes(&base, &their, &our, "")
	if len(conflicts) > 0 {
		return &Merge3WayOutput{Content: nil, Conflicts: conflicts}, nil
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
	return &Merge3WayOutput{Content: buf.Bytes(), Conflicts: nil}, nil
}

func merge3WayYAMLNodes(base, their, our *yaml.Node, path string) (*yaml.Node, []MergeConflict) {
	if base.Kind == yaml.DocumentNode && their.Kind == yaml.DocumentNode && our.Kind == yaml.DocumentNode {
		merged, conflicts := merge3WayYAMLNodes(base.Content[0], their.Content[0], our.Content[0], path)
		out := *base
		out.Content = []*yaml.Node{merged}
		return &out, conflicts
	}
	switch {
	case their.Kind == yaml.MappingNode && our.Kind == yaml.MappingNode:
		baseMapNode := base
		if baseMapNode.Kind != yaml.MappingNode {
			baseMapNode = nil
		}
		return merge3WayYAMLMapping(baseMapNode, their, our, path)
	case their.Kind == yaml.SequenceNode && our.Kind == yaml.SequenceNode:
		baseSeqNode := base
		if baseSeqNode.Kind != yaml.SequenceNode {
			baseSeqNode = nil
		}
		return merge3WayYAMLSequence(baseSeqNode, their, our, path)
	default:
		return merge3WayYAMLScalar(base, their, our, path)
	}
}

func merge3WayYAMLMapping(base, their, our *yaml.Node, path string) (*yaml.Node, []MergeConflict) {
	baseMap := yamlMappingToIndex(base)
	theirMap := yamlMappingToIndex(their)
	ourMap := yamlMappingToIndex(our)
	keys := orderedMappingKeys(their, our)
	out := *their
	out.Content = nil
	var conflicts []MergeConflict
	for _, k := range keys {
		childPath := joinPath(path, k)
		theirIdx, theirHas := theirMap[k]
		ourIdx, ourHas := ourMap[k]
		baseIdx, baseHas := baseMap[k]
		switch {
		case theirHas && ourHas:
			var baseNode *yaml.Node
			if baseHas {
				baseNode = base.Content[baseIdx+1]
			}
			merged, childConflicts := mergeOrFallback(baseNode, their.Content[theirIdx+1], our.Content[ourIdx+1], childPath)
			out.Content = append(out.Content, scalarKeyNode(k), merged)
			conflicts = append(conflicts, childConflicts...)
		case theirHas && !ourHas:
			if !baseHas || nodesEqual(base.Content[baseIdx+1], their.Content[theirIdx+1]) {
				continue
			}
			out.Content = append(out.Content, scalarKeyNode(k), their.Content[theirIdx+1])
		case !theirHas && ourHas:
			if !baseHas || nodesEqual(base.Content[baseIdx+1], our.Content[ourIdx+1]) {
				out.Content = append(out.Content, scalarKeyNode(k), our.Content[ourIdx+1])
				continue
			}
			conflicts = append(conflicts, MergeConflict{Path: childPath, Description: "their (vendor) removed; our (overlay) kept with modification"})
			out.Content = append(out.Content, scalarKeyNode(k), our.Content[ourIdx+1])
		}
	}
	return &out, conflicts
}

func mergeOrFallback(base, their, our *yaml.Node, path string) (*yaml.Node, []MergeConflict) {
	return merge3WayYAMLNodes(base, their, our, path)
}

func merge3WayYAMLSequence(base, their, our *yaml.Node, path string) (*yaml.Node, []MergeConflict) {
	baseKeys := yamlSequenceKeys(base)
	theirKeys := yamlSequenceKeys(their)
	ourKeys := yamlSequenceKeys(our)
	var conflicts []MergeConflict
	if base != nil {
		for k := range baseKeys {
			_, inTheir := theirKeys[k]
			_, inOur := ourKeys[k]
			if !inTheir && inOur {
				conflicts = append(conflicts, MergeConflict{Path: path + "[" + k + "]", Description: "their (vendor) removed array element; our (overlay) kept"})
			}
		}
	}
	if len(conflicts) > 0 {
		return their, conflicts
	}
	out := *their
	out.Content = nil
	seen := map[string]struct{}{}
	userRemoved := map[string]struct{}{}
	if base != nil {
		for k := range baseKeys {
			if _, ok := ourKeys[k]; !ok {
				userRemoved[k] = struct{}{}
			}
		}
	}
	for _, n := range their.Content {
		k := yamlNodeKey(n)
		if _, removed := userRemoved[k]; removed {
			continue
		}
		seen[k] = struct{}{}
		out.Content = append(out.Content, n)
	}
	for _, n := range our.Content {
		k := yamlNodeKey(n)
		if _, dup := seen[k]; dup {
			continue
		}
		out.Content = append(out.Content, n)
	}
	return &out, nil
}

func merge3WayYAMLScalar(base, their, our *yaml.Node, path string) (*yaml.Node, []MergeConflict) {
	if nodesEqual(their, our) {
		return their, nil
	}
	if base == nil || base.Kind == 0 {
		return nil, []MergeConflict{{Path: path, Description: fmt.Sprintf("their=%q our=%q (no base)", scalarValue(their), scalarValue(our))}}
	}
	if nodesEqual(base, their) {
		return our, nil
	}
	if nodesEqual(base, our) {
		return their, nil
	}
	return nil, []MergeConflict{{Path: path, Description: fmt.Sprintf("base=%q their=%q our=%q", scalarValue(base), scalarValue(their), scalarValue(our))}}
}

func yamlMappingToIndex(n *yaml.Node) map[string]int {
	out := map[string]int{}
	if n == nil || n.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(n.Content); i += keyValueStep {
		out[n.Content[i].Value] = i
	}
	return out
}

func orderedMappingKeys(their, our *yaml.Node) []string {
	seen := map[string]struct{}{}
	var keys []string
	for _, n := range []*yaml.Node{their, our} {
		if n == nil || n.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(n.Content); i += keyValueStep {
			k := n.Content[i].Value
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	return keys
}

func yamlSequenceKeys(n *yaml.Node) map[string]struct{} {
	out := map[string]struct{}{}
	if n == nil || n.Kind != yaml.SequenceNode {
		return out
	}
	for _, c := range n.Content {
		out[yamlNodeKey(c)] = struct{}{}
	}
	return out
}

func yamlNodeKey(n *yaml.Node) string {
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

func nodesEqual(a, b *yaml.Node) bool {
	return yamlNodeKey(a) == yamlNodeKey(b)
}

func scalarValue(n *yaml.Node) string {
	if n == nil {
		return "<nil>"
	}
	return n.Value
}

func scalarKeyNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "", Value: s, Anchor: "", Alias: nil, Content: nil, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: 0}
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func merge3WayJSON(in *Merge3WayInput) (*Merge3WayOutput, error) {
	var base, their, our any
	if err := json.Unmarshal(in.BaseContent, &base); err != nil {
		return nil, ex.Wrapf(err, "parse base json: %s", in.InstallPath)
	}
	if err := json.Unmarshal(in.TheirContent, &their); err != nil {
		return nil, ex.Wrapf(err, "parse their json: %s", in.InstallPath)
	}
	if err := json.Unmarshal(in.OurContent, &our); err != nil {
		return nil, ex.Wrapf(err, "parse our json: %s", in.InstallPath)
	}
	merged, conflicts := merge3WayJSONValue(base, their, our, "")
	if len(conflicts) > 0 {
		return &Merge3WayOutput{Content: nil, Conflicts: conflicts}, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(merged); err != nil {
		return nil, ex.Wrap(err)
	}
	return &Merge3WayOutput{Content: buf.Bytes(), Conflicts: nil}, nil
}

func merge3WayJSONValue(base, their, our any, path string) (any, []MergeConflict) {
	tMap, tIsMap := their.(map[string]any)
	oMap, oIsMap := our.(map[string]any)
	if tIsMap && oIsMap {
		bMap, _ := base.(map[string]any)
		return merge3WayJSONMap(bMap, tMap, oMap, path)
	}
	tArr, tIsArr := their.([]any)
	oArr, oIsArr := our.([]any)
	if tIsArr && oIsArr {
		bArr, _ := base.([]any)
		return merge3WayJSONArray(bArr, tArr, oArr, path)
	}
	return merge3WayJSONScalar(base, their, our, path)
}

func merge3WayJSONMap(base, their, our map[string]any, path string) (map[string]any, []MergeConflict) {
	out := map[string]any{}
	keys := jsonMapKeys(their, our)
	var conflicts []MergeConflict
	for _, k := range keys {
		childPath := joinPath(path, k)
		tv, theirHas := their[k]
		ov, ourHas := our[k]
		bv, baseHas := base[k]
		switch {
		case theirHas && ourHas:
			merged, childConflicts := merge3WayJSONValue(bv, tv, ov, childPath)
			out[k] = merged
			conflicts = append(conflicts, childConflicts...)
		case theirHas && !ourHas:
			if !baseHas || jsonValuesEqual(bv, tv) {
				continue
			}
			out[k] = tv
		case !theirHas && ourHas:
			if !baseHas || jsonValuesEqual(bv, ov) {
				out[k] = ov
				continue
			}
			conflicts = append(conflicts, MergeConflict{Path: childPath, Description: "their (vendor) removed; our (overlay) kept with modification"})
			out[k] = ov
		}
	}
	return out, conflicts
}

func merge3WayJSONArray(base, their, our []any, path string) ([]any, []MergeConflict) {
	baseSet := jsonArraySet(base)
	theirSet := jsonArraySet(their)
	ourSet := jsonArraySet(our)
	var conflicts []MergeConflict
	for k := range baseSet {
		_, inTheir := theirSet[k]
		_, inOur := ourSet[k]
		if !inTheir && inOur {
			conflicts = append(conflicts, MergeConflict{Path: path + "[" + k + "]", Description: "their (vendor) removed array element; our (overlay) kept"})
		}
	}
	if len(conflicts) > 0 {
		return their, conflicts
	}
	userRemoved := map[string]struct{}{}
	for k := range baseSet {
		if _, ok := ourSet[k]; !ok {
			userRemoved[k] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	out := []any{}
	for _, v := range their {
		k := jsonValueKey(v)
		if _, removed := userRemoved[k]; removed {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	for _, v := range our {
		k := jsonValueKey(v)
		if _, dup := seen[k]; dup {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func merge3WayJSONScalar(base, their, our any, path string) (any, []MergeConflict) {
	if jsonValuesEqual(their, our) {
		return their, nil
	}
	if base == nil {
		return nil, []MergeConflict{{Path: path, Description: fmt.Sprintf("their=%v our=%v (no base)", their, our)}}
	}
	if jsonValuesEqual(base, their) {
		return our, nil
	}
	if jsonValuesEqual(base, our) {
		return their, nil
	}
	return nil, []MergeConflict{{Path: path, Description: fmt.Sprintf("base=%v their=%v our=%v", base, their, our)}}
}

func jsonMapKeys(their, our map[string]any) []string {
	seen := map[string]struct{}{}
	var keys []string
	for k := range their {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	for k := range our {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	return keys
}

func jsonArraySet(arr []any) map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range arr {
		out[jsonValueKey(v)] = struct{}{}
	}
	return out
}

func jsonValueKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func jsonValuesEqual(a, b any) bool {
	return jsonValueKey(a) == jsonValueKey(b)
}

// mergeLines3Way は base / their / our を line 単位で 3-way merge する。 conflict 検出は「base に存在し their と our 双方で異なる変更を受けている」 場合に限定する。 簡易版: line を集合として扱い、 their と our の両方の追加を取り込み、 base から両方が削除した行は除外する (= line set merge)。 順序保持はあきらめ、 their の出現順を base に対する位置として優先する。
func mergeLines3Way(base, their, our []byte) ([]byte, []MergeConflict) {
	baseLines := splitLines(base)
	theirLines := splitLines(their)
	ourLines := splitLines(our)
	baseSet := stringSet(baseLines)
	theirSet := stringSet(theirLines)
	ourSet := stringSet(ourLines)
	var conflicts []MergeConflict
	for line := range baseSet {
		_, inTheir := theirSet[line]
		_, inOur := ourSet[line]
		if !inTheir && inOur {
			conflicts = append(conflicts, MergeConflict{Path: "(line)", Description: fmt.Sprintf("their (vendor) removed line %q; our (overlay) kept", line)})
		}
	}
	if len(conflicts) > 0 {
		return nil, conflicts
	}
	userRemoved := map[string]struct{}{}
	for line := range baseSet {
		if _, ok := ourSet[line]; !ok {
			userRemoved[line] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	var merged []string
	for _, line := range theirLines {
		if _, removed := userRemoved[line]; removed {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		merged = append(merged, line)
	}
	for _, line := range ourLines {
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		merged = append(merged, line)
	}
	return joinLines(merged, len(their) > 0 && their[len(their)-1] == '\n'), nil
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s, _ := strings.CutSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func stringSet(ss []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		out[s] = struct{}{}
	}
	return out
}

func joinLines(lines []string, trailingNewline bool) []byte {
	if len(lines) == 0 {
		if trailingNewline {
			return []byte("\n")
		}
		return nil
	}
	out := strings.Join(lines, "\n")
	if trailingNewline {
		out += "\n"
	}
	return []byte(out)
}
