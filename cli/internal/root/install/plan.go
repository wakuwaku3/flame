package install

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wakuwaku3/flame/lib/ex"
)

// 共通権限定数。 fsperm の owner-only 値は機密相当 file 用 (= release artifact) の経路を想定しているため install 経路では別 const を使う。 magic number 出現は本ファイルに集約することで mnd / gosec G306 の検出を 1 か所に閉じる (FLM_GEN_0006)。
const (
	filePerm        fs.FileMode = 0o644
	readOnlyPerm    fs.FileMode = 0o444
	dirPerm         fs.FileMode = 0o755
	readOnlyDirPerm fs.FileMode = 0o555
	yamlIndent                  = 2
)

// VendorRoot は flame harness vendor SoT の repo root 相対 path。
const VendorRoot = "vendor/flame"

// flameTrgPrefix は GitHub Actions トリガー層 workflow を install 先で命名する prefix (FLM_FEA_0003 §workflow の install 命名規約)。
const flameTrgPrefix = "flame-"

const githubWorkflowsDir = ".github/workflows"

// EmbedRule は repo root 取り込み形式 (CLAUDE.md / .envrc / .yamllint) の組み合わせ (FLM_GEN_0007 §repo root における downstream resource の取り込み形式)。
type EmbedRule struct {
	VendorPath string
	Snippet    string
}

// embedRules は取り込み形式の対象 path 列。 個別 file の存在は plan 時に確認する。
var embedRules = []EmbedRule{
	{
		VendorPath: filepath.Join(VendorRoot, "CLAUDE.md"),
		Snippet:    "[vendor/flame/CLAUDE.md](vendor/flame/CLAUDE.md)\n",
	},
	{
		VendorPath: filepath.Join(VendorRoot, ".envrc"),
		Snippet:    "source_env_if_exists vendor/flame/.envrc\n",
	},
	{
		VendorPath: filepath.Join(VendorRoot, ".yamllint"),
		Snippet:    "extends: vendor/flame/.yamllint\n",
	},
}

// PlanItem は install 1 entry を表す。 install copy / trigger workflow / embed のいずれか。 Kind は最初に置いて fieldalignment を満たす。
type PlanItem struct {
	VendorPath  string
	InstallPath string
	Merge       MergeStrategy
	Kind        PlanKind
}

type PlanKind int

const (
	// PlanKindUnknown は zero value (使用されない)。
	PlanKindUnknown PlanKind = iota
	// PlanKindInstallCopy は flame.lock.files[] に登録される install copy。
	PlanKindInstallCopy
	// PlanKindTriggerWorkflow は `.github/workflows/flame-trg__*.yaml` の scaffold (lock 対象外)。
	PlanKindTriggerWorkflow
	// PlanKindEmbed は repo root の取り込み snippet 経路 (lock embeds[])。
	PlanKindEmbed
)

type Plan struct {
	Items []PlanItem
}

// BuildPlan は repo root と manifest を受けて vendor SoT を walk し、 各 file を classify した Plan を返す。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func BuildPlan(_ context.Context, repoRoot string, m *Manifest) (*Plan, error) {
	vendorAbs := filepath.Join(repoRoot, VendorRoot)
	if _, err := os.Stat(vendorAbs); err != nil {
		return nil, ex.Wrapf(err, "vendor root not found: %s", vendorAbs)
	}
	embedSet := make(map[string]struct{}, len(embedRules))
	for _, e := range embedRules {
		embedSet[filepath.ToSlash(e.VendorPath)] = struct{}{}
	}
	var items []PlanItem
	walkErr := filepath.WalkDir(vendorAbs, func(absPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return ex.Wrap(err)
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return ex.Wrap(err)
		}
		relSlash := filepath.ToSlash(rel)
		if _, ok := embedSet[relSlash]; ok {
			return nil
		}
		if skipVendorPath(relSlash, m.IsSelf()) {
			return nil
		}
		installRel, kind := resolveInstallPath(relSlash)
		if kind == PlanKindUnknown {
			return nil
		}
		strategy := defaultMergeStrategy(installRel)
		items = append(items, PlanItem{
			VendorPath:  rel,
			InstallPath: installRel,
			Merge:       strategy,
			Kind:        kind,
		})
		return nil
	})
	if walkErr != nil {
		return nil, ex.Wrap(walkErr)
	}
	for _, e := range embedRules {
		vendorAbsEmbed := filepath.Join(repoRoot, e.VendorPath)
		if _, statErr := os.Stat(vendorAbsEmbed); statErr != nil {
			continue
		}
		items = append(items, PlanItem{
			VendorPath:  e.VendorPath,
			InstallPath: snippetInstallPath(e.VendorPath),
			Merge:       "",
			Kind:        PlanKindEmbed,
		})
	}
	return &Plan{Items: items}, nil
}

func snippetInstallPath(vendorRel string) string {
	rel, err := filepath.Rel(VendorRoot, vendorRel)
	if err != nil {
		return ""
	}
	return rel
}

// resolveInstallPath は vendor 配下 path から repo root 相対の install 先 path を導く。 `.github/workflows/trg__*.yaml` は flame- prefix を付ける (FLM_FEA_0003 §workflow の install 命名規約)。
func resolveInstallPath(vendorRel string) (installPath string, kind PlanKind) {
	relUnderVendor := strings.TrimPrefix(vendorRel, VendorRoot+"/")
	if relUnderVendor == vendorRel {
		return "", PlanKindUnknown
	}
	if strings.HasPrefix(relUnderVendor, githubWorkflowsDir+"/") {
		base := filepath.Base(relUnderVendor)
		if strings.HasPrefix(base, "trg__") && strings.HasSuffix(base, ".yaml") {
			return filepath.Join(githubWorkflowsDir, flameTrgPrefix+base), PlanKindTriggerWorkflow
		}
		return "", PlanKindUnknown
	}
	return relUnderVendor, PlanKindInstallCopy
}

func skipVendorPath(vendorRel string, isSelf bool) bool {
	relUnderVendor := strings.TrimPrefix(vendorRel, VendorRoot+"/")
	if relUnderVendor == vendorRel {
		return true
	}
	switch {
	case relUnderVendor == "devbox.lock":
		return true
	case strings.HasPrefix(relUnderVendor, ".github/workflows/tests/"):
		return true
	case strings.HasPrefix(relUnderVendor, "devbox/"):
		return true
	case strings.HasPrefix(relUnderVendor, "schemas/"):
		// schemas/ は flame install の install 経路を取らず、 利用側 / source 提供元 repo の双方が vendor SoT を直接参照する (FLM_FEA_0003 §schema の機械可読化、 `tests/shared/` と同じ運用)。
		return true
	}
	if isSelf {
		switch {
		case strings.HasPrefix(relUnderVendor, "docs/adr/"):
			return true
		case strings.HasPrefix(relUnderVendor, ".claude/rules/"):
			return true
		}
	}
	return false
}

// defaultMergeStrategy は install path の拡張子から default merge strategy を導く (FLM_FEA_0003 §副ファイル overlay 機構)。
func defaultMergeStrategy(installPath string) MergeStrategy {
	ext := strings.ToLower(filepath.Ext(installPath))
	switch ext {
	case ".yaml", ".yml", ".json":
		return MergeDeep
	case ".md", ".sh":
		return MergeAppend
	case "":
		return MergeAppend
	default:
		return MergeAppend
	}
}
