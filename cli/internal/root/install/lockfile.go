package install

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/wakuwaku3/flame/lib/ex"
)

// MergeStrategy は flame.lock.files[].merge の取り得る値を表す。 拡張子からの推論には依存せず lockfile 単体で挙動を決定可能にするため、 全 entry で必須記録される (FLM_FEA_0003 §動的マージ対象ファイル)。
type MergeStrategy string

const (
	MergeDeep    MergeStrategy = "deep"
	MergeAppend  MergeStrategy = "append"
	MergeReplace MergeStrategy = "replace"
)

// LockOverlay は副ファイル overlay の lockfile 表現。 path に加え 3-way merge の base material として前回 install 時点の overlay 内容 snapshot を持つ (FLM_FEA_0003 §flame.lock)。
type LockOverlay struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content,omitempty"`
}

// LockInstalled は前回 install 実行時の vendor 取得元 / version / vendor tree hash を記録する (FLM_FEA_0003 §flame.lock)。 version 変更検知 (= 再 fetch の起動条件) と vendor 改変検知 (= drift 検知) のために用いる。 self mode では tree_hash は記録しない (working tree が常時変動するため CI が壊れる)。
type LockInstalled struct {
	Source   string `yaml:"source"`
	Version  string `yaml:"version"`
	TreeHash string `yaml:"tree_hash,omitempty"`
}

// LockFile は flame.lock.files[] の 1 entry。 install copy 経路で配置された各ファイルのレコード。 vendor_content は 3-way merge の base (= 前回 install 時点の vendor file 内容 snapshot) として用いる。 fieldalignment 最適化のため pointer (Overlay) を先頭に置き GC scan 範囲を縮減する。
type LockFile struct {
	Overlay       *LockOverlay  `yaml:"overlay,omitempty"`
	Install       string        `yaml:"install"`
	Vendor        string        `yaml:"vendor"`
	Merge         MergeStrategy `yaml:"merge"`
	Content       string        `yaml:"content"`
	VendorContent string        `yaml:"vendor_content,omitempty"`
}

// LockEmbed は flame.lock.embeds[] の 1 entry。 取り込み形式 (CLAUDE.md / .envrc / .yamllint) の install 先と vendor target / 取り込み snippet を記録する (FLM_GEN_0007 §repo root における downstream resource の取り込み形式)。
type LockEmbed struct {
	Install string `yaml:"install"`
	Target  string `yaml:"target"`
	Snippet string `yaml:"snippet"`
}

type Lock struct {
	Installed *LockInstalled
	Files     []LockFile
	Embeds    []LockEmbed
}

type lockFileYAML struct {
	Flame lockRootYAML `yaml:"flame"`
}

type lockRootYAML struct {
	Installed *LockInstalled `yaml:"installed,omitempty"`
	Files     []LockFile     `yaml:"files"`
	Embeds    []LockEmbed    `yaml:"embeds,omitempty"`
}

// LoadLock は repo root の flame.lock を読み取って Lock を返す。 file が無い場合は空 Lock を返す (初回 install で許容する)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func LoadLock(_ context.Context, repoRoot string) (*Lock, error) {
	path := filepath.Join(repoRoot, "flame.lock")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Lock{Installed: nil, Files: nil, Embeds: nil}, nil
	}
	if err != nil {
		return nil, ex.Wrapf(err, "read flame.lock: %s", path)
	}
	var raw lockFileYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, ex.Wrapf(err, "parse flame.lock: %s", path)
	}
	return &Lock{
		Installed: raw.Flame.Installed,
		Files:     raw.Flame.Files,
		Embeds:    raw.Flame.Embeds,
	}, nil
}

// WriteLock は flame.lock を repo root に書き出す。 yaml.v3 の Encoder で indent=2 + literal-block style を維持する。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func WriteLock(_ context.Context, repoRoot string, lock *Lock) error {
	path := filepath.Join(repoRoot, "flame.lock")
	header := "---\n# flame harness lockfile (FLM_FEA_0003、 CLI 自動生成・更新)\n"
	body, err := encodeLock(lock)
	if err != nil {
		return err
	}
	out := header + body
	if err := os.WriteFile(path, []byte(out), filePerm); err != nil {
		return ex.Wrapf(err, "write flame.lock: %s", path)
	}
	return nil
}

func encodeLock(lock *Lock) (string, error) {
	root := buildLockNode(lock)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(root); err != nil {
		return "", ex.Wrap(err)
	}
	if err := enc.Close(); err != nil {
		return "", ex.Wrap(err)
	}
	return buf.String(), nil
}

// buildLockNode は yaml.Node を手組みし、 content を literal block scalar (`|`) で出力するために node style を強制する。
func buildLockNode(lock *Lock) *yaml.Node {
	flameFields := []*yaml.Node{}
	if lock.Installed != nil {
		flameFields = append(flameFields, scalarNode("installed"), buildInstalledNode(lock.Installed))
	}
	files := &yaml.Node{Kind: yaml.SequenceNode, Tag: "", Value: "", Anchor: "", Alias: nil, Content: nil, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: 0}
	for i := range lock.Files {
		files.Content = append(files.Content, buildFileNode(&lock.Files[i]))
	}
	flameFields = append(flameFields, scalarNode("files"), files)
	if len(lock.Embeds) > 0 {
		embeds := &yaml.Node{Kind: yaml.SequenceNode, Tag: "", Value: "", Anchor: "", Alias: nil, Content: nil, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: 0}
		for i := range lock.Embeds {
			embeds.Content = append(embeds.Content, buildEmbedNode(&lock.Embeds[i]))
		}
		flameFields = append(flameFields, scalarNode("embeds"), embeds)
	}
	flame := mappingNode(flameFields)
	return mappingNode([]*yaml.Node{scalarNode("flame"), flame})
}

func buildInstalledNode(installed *LockInstalled) *yaml.Node {
	fields := []*yaml.Node{
		scalarNode("source"), scalarNode(installed.Source),
		scalarNode("version"), scalarNode(installed.Version),
	}
	if installed.TreeHash != "" {
		fields = append(fields, scalarNode("tree_hash"), scalarNode(installed.TreeHash))
	}
	return mappingNode(fields)
}

func buildFileNode(f *LockFile) *yaml.Node {
	fields := []*yaml.Node{
		scalarNode("install"), scalarNode(f.Install),
		scalarNode("vendor"), scalarNode(f.Vendor),
		scalarNode("merge"), scalarNode(string(f.Merge)),
		scalarNode("content"), literalBlockNode(f.Content),
	}
	if f.VendorContent != "" {
		fields = append(fields, scalarNode("vendor_content"), literalBlockNode(f.VendorContent))
	}
	if f.Overlay != nil {
		overlayFields := []*yaml.Node{scalarNode("path"), scalarNode(f.Overlay.Path)}
		if f.Overlay.Content != "" {
			overlayFields = append(overlayFields, scalarNode("content"), literalBlockNode(f.Overlay.Content))
		}
		fields = append(fields, scalarNode("overlay"), mappingNode(overlayFields))
	}
	return mappingNode(fields)
}

func buildEmbedNode(e *LockEmbed) *yaml.Node {
	return mappingNode([]*yaml.Node{
		scalarNode("install"), scalarNode(e.Install),
		scalarNode("target"), scalarNode(e.Target),
		scalarNode("snippet"), literalBlockNode(e.Snippet),
	})
}

func scalarNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "", Value: s, Anchor: "", Alias: nil, Content: nil, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: 0}
}

func literalBlockNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "", Value: s, Anchor: "", Alias: nil, Content: nil, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: yaml.LiteralStyle}
}

func mappingNode(content []*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "", Value: "", Anchor: "", Alias: nil, Content: content, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: 0}
}

func FormatStrategySummary(lock *Lock) string {
	return fmt.Sprintf("files=%d embeds=%d", len(lock.Files), len(lock.Embeds))
}
