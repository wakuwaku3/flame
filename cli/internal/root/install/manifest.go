package install

import (
	"context"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/ex"
)

// SelfVersion は flame harness の source 提供元 repo (= flame self) を示す flame.yaml の特別 marker。 release artifact 取得経路ではなく working tree の vendor SoT を直接コピーする経路を CLI に取らせる (FLM_FEA_0003 §flame.yaml manifest)。
const SelfVersion = "self"

// IgnoreGitignore は `flame install` の `.gitignore` 自動登録工程を skip する marker。 flame self は vendor/flame/ を commit する責務があるため (FLM_FEA_0003 §flame.yaml manifest)。
const IgnoreGitignore = ".gitignore"

// IgnorePluginInstall は Claude Code plugin marketplace 登録 + plugin install の自動工程を skip する marker。 flame self は repo 自身が plugin の source 提供元のため (FLM_FEA_0003 §flame.yaml manifest)。
const IgnorePluginInstall = ".claude/plugins"

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
	Ignore            map[string]struct{}
	Stage1ExtraAgents []string
}

// LoadManifest は repo root の flame.yaml を読み取って Manifest を返す。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
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
	ignore := make(map[string]struct{}, len(raw.Flame.Ignore))
	for _, k := range raw.Flame.Ignore {
		ignore[k] = struct{}{}
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

// SkipGitignore は `.gitignore` 自動登録工程を skip すべきかを返す。
func (m *Manifest) SkipGitignore() bool {
	_, ok := m.Ignore[IgnoreGitignore]
	return ok
}

// SkipPluginInstall は Claude Code plugin marketplace 登録 + plugin install を skip すべきかを返す。
func (m *Manifest) SkipPluginInstall() bool {
	_, ok := m.Ignore[IgnorePluginInstall]
	return ok
}
