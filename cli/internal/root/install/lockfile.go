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
	// MergeDeep は構造化 deep merge (YAML / JSON)。
	MergeDeep MergeStrategy = "deep"
	// MergeAppend はテキスト末尾追加 (vendor 末尾に overlay を連結)。
	MergeAppend MergeStrategy = "append"
	// MergeReplace は overlay 不可で vendor のみで上書き (生成物に使う)。
	MergeReplace MergeStrategy = "replace"
)

// MergeArrayStrategy は構造化 deep merge 時の配列扱い。
type MergeArrayStrategy string

const (
	// MergeArrayAppend は vendor 末尾に overlay を連結。
	MergeArrayAppend MergeArrayStrategy = "append"
	// MergeArrayReplace は overlay の配列で全置換。
	MergeArrayReplace MergeArrayStrategy = "replace"
	// MergeArrayUnique は append + 重複除去。
	MergeArrayUnique MergeArrayStrategy = "unique"
)

// LockOverlay は副ファイル overlay の lockfile 表現。 内容は副ファイル自体が SoT のため path のみ記録する (FLM_FEA_0003 §flame.lock)。
type LockOverlay struct {
	Path string `yaml:"path"`
}

// LockFile は flame.lock.files[] の 1 entry。 install copy 経路で配置された各ファイルのレコード。
type LockFile struct {
	Install    string             `yaml:"install"`
	Vendor     string             `yaml:"vendor"`
	Merge      MergeStrategy      `yaml:"merge"`
	Content    string             `yaml:"content"`
	Overlay    *LockOverlay       `yaml:"overlay,omitempty"`
	MergeArray MergeArrayStrategy `yaml:"merge_array,omitempty"`
}

// LockEmbed は flame.lock.embeds[] の 1 entry。 取り込み形式 (CLAUDE.md / .envrc / .yamllint) の install 先と vendor target / 取り込み snippet を記録する (FLM_GEN_0007 §repo root における downstream resource の取り込み形式)。
type LockEmbed struct {
	Install string `yaml:"install"`
	Target  string `yaml:"target"`
	Snippet string `yaml:"snippet"`
}

// Lock は flame.lock 全体を表す。
type Lock struct {
	Files  []LockFile
	Embeds []LockEmbed
}

type lockFileYAML struct {
	Flame lockRootYAML `yaml:"flame"`
}

type lockRootYAML struct {
	Harness lockHarnessYAML `yaml:"harness"`
}

type lockHarnessYAML struct {
	Files  []LockFile  `yaml:"files"`
	Embeds []LockEmbed `yaml:"embeds,omitempty"`
}

// LoadLock は repo root の flame.lock を読み取って Lock を返す。 file が無い場合は空 Lock を返す (初回 install で許容する)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func LoadLock(_ context.Context, repoRoot string) (*Lock, error) {
	path := filepath.Join(repoRoot, "flame.lock")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Lock{Files: nil, Embeds: nil}, nil
	}
	if err != nil {
		return nil, ex.Wrapf(err, "read flame.lock: %s", path)
	}
	var raw lockFileYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, ex.Wrapf(err, "parse flame.lock: %s", path)
	}
	return &Lock{
		Files:  raw.Flame.Harness.Files,
		Embeds: raw.Flame.Harness.Embeds,
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
	files := &yaml.Node{Kind: yaml.SequenceNode, Tag: "", Value: "", Anchor: "", Alias: nil, Content: nil, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: 0}
	for i := range lock.Files {
		files.Content = append(files.Content, buildFileNode(&lock.Files[i]))
	}
	harnessFields := []*yaml.Node{
		scalarNode("files"),
		files,
	}
	if len(lock.Embeds) > 0 {
		embeds := &yaml.Node{Kind: yaml.SequenceNode, Tag: "", Value: "", Anchor: "", Alias: nil, Content: nil, HeadComment: "", LineComment: "", FootComment: "", Line: 0, Column: 0, Style: 0}
		for i := range lock.Embeds {
			embeds.Content = append(embeds.Content, buildEmbedNode(&lock.Embeds[i]))
		}
		harnessFields = append(harnessFields, scalarNode("embeds"), embeds)
	}
	harness := mappingNode(harnessFields)
	flame := mappingNode([]*yaml.Node{scalarNode("harness"), harness})
	return mappingNode([]*yaml.Node{scalarNode("flame"), flame})
}

func buildFileNode(f *LockFile) *yaml.Node {
	fields := []*yaml.Node{
		scalarNode("install"), scalarNode(f.Install),
		scalarNode("vendor"), scalarNode(f.Vendor),
		scalarNode("merge"), scalarNode(string(f.Merge)),
		scalarNode("content"), literalBlockNode(f.Content),
	}
	if f.Overlay != nil {
		overlayNode := mappingNode([]*yaml.Node{scalarNode("path"), scalarNode(f.Overlay.Path)})
		fields = append(fields, scalarNode("overlay"), overlayNode)
	}
	if f.MergeArray != "" {
		fields = append(fields, scalarNode("merge_array"), scalarNode(string(f.MergeArray)))
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

// FormatStrategySummary は debug 出力 / status 表示用の summary 文字列を組み立てる。
func FormatStrategySummary(lock *Lock) string {
	return fmt.Sprintf("files=%d embeds=%d", len(lock.Files), len(lock.Embeds))
}
