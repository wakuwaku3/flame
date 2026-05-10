package install

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/ex"
)

// SelfVersion は flame harness の source 提供元 repo (= flame self) を示す flame.yaml の特別 marker。 release artifact 取得経路ではなく working tree の vendor SoT を直接コピーする経路を CLI に取らせる (FLM_FEA_0003 §flame.yaml manifest)。
const SelfVersion = "self"

type manifestFile struct {
	Flame manifestRoot `yaml:"flame"`
}

type manifestRoot struct {
	Source  string     `yaml:"source"`
	Version string     `yaml:"version"`
	Ignore  []string   `yaml:"ignore,omitempty"`
	AI      manifestAI `yaml:"ai,omitempty"`
}

type manifestAI struct {
	PrePush manifestPrePush `yaml:"pre_push,omitempty"`
}

type manifestPrePush struct {
	Stage1ExtraAgents []string `yaml:"stage1_extra_agents,omitempty"`
}

// Manifest は flame.yaml を読み込んだ後の値オブジェクト。
type Manifest struct {
	Source            string
	Version           string
	Ignore            map[Feature]struct{}
	Stage1ExtraAgents []string
}

// LoadManifest は repo root の flame.yaml を読み取って Manifest を返す。 ignore に未知の Feature 値が含まれていれば error にする (typo / 古い ID 名を即時検出する)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func LoadManifest(_ context.Context, repoRoot string) (*Manifest, error) {
	path := filepath.Join(repoRoot, "flame.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ex.Wrapf(err, "read flame.yaml: %s", path)
	}
	var raw manifestFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, ex.Wrapf(err, "parse flame.yaml: %s", path)
	}
	if raw.Flame.Source == "" {
		return nil, ex.Errorf("flame.yaml: flame.source is required")
	}
	if raw.Flame.Version == "" {
		return nil, ex.Errorf("flame.yaml: flame.version is required")
	}
	known := knownFeatureSet()
	ignore := make(map[Feature]struct{}, len(raw.Flame.Ignore))
	for _, k := range raw.Flame.Ignore {
		f := Feature(k)
		if _, ok := known[f]; !ok {
			return nil, ex.Errorf("flame.yaml: unknown ignore feature %q (known: %v)", k, sortedFeatureNames())
		}
		ignore[f] = struct{}{}
	}
	return &Manifest{
		Source:            raw.Flame.Source,
		Version:           raw.Flame.Version,
		Ignore:            ignore,
		Stage1ExtraAgents: append([]string(nil), raw.Flame.AI.PrePush.Stage1ExtraAgents...),
	}, nil
}

// IsSelf は当該 repo が harness の source 提供元 (= flame self) かを判定する。
func (m *Manifest) IsSelf() bool {
	return m.Version == SelfVersion
}

// IsIgnored は manifest の ignore に当該 Feature が含まれているかを返す。
func (m *Manifest) IsIgnored(f Feature) bool {
	if f == "" {
		return false
	}
	_, ok := m.Ignore[f]
	return ok
}

func knownFeatureSet() map[Feature]struct{} {
	all := AllFeatures()
	out := make(map[Feature]struct{}, len(all))
	for _, f := range all {
		out[f] = struct{}{}
	}
	return out
}

func sortedFeatureNames() []string {
	all := AllFeatures()
	names := make([]string, len(all))
	for i, f := range all {
		names[i] = string(f)
	}
	sort.Strings(names)
	return names
}
