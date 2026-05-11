package install

import (
	"path/filepath"
	"strings"
)

// Feature は flame install の skip 単位を表す機能 ID (FLM_FEA_0003 §flame.yaml manifest §機能単位 ignore)。 利用側 manifest の `flame.ignore` で値を列挙し、 該当機能の install 工程を skip する。 値の命名は (1) tool 名 (kebab-case): `golangci-lint`、 (2) path-like (resource hierarchy): `claude/rules`、 (3) 工程名: `vendor-sync` を組み合わせる。
type Feature string

const (
	// 工程単位 (= path に紐づかない処理を skip する).
	FeatureVendorSync      Feature = "vendor-sync"
	FeatureVendorReadonly  Feature = "vendor-readonly"
	FeatureReadOnly        Feature = "read-only"
	FeatureGitignore       Feature = "gitignore"
	FeatureClaudePlugins   Feature = "claude/plugins"
	FeatureTriggerWorkflow Feature = "trigger-workflow"

	// resource type / tool 単位 (= 該当 install path 群を skip する).
	FeatureClaudeRules  Feature = "claude/rules"
	FeatureClaudeSkills Feature = "claude/skills"
	FeatureGolangciLint Feature = "golangci-lint"
	FeatureMarkdownLint Feature = "markdown-lint"
	FeatureShellcheck   Feature = "shellcheck"
	FeatureDevbox       Feature = "devbox"
	FeatureVscode       Feature = "vscode"
	FeatureADR          Feature = "adr"

	// embed (= 取り込み形式の snippet 注入を skip する).
	FeatureEmbedClaudeMd Feature = "embed/claude-md"
	FeatureEmbedEnvrc    Feature = "embed/envrc"
	FeatureEmbedYamllint Feature = "embed/yamllint"
)

// AllFeatures は manifest.ignore の validation 用に既知の Feature 列を返す。 manifest 側で未知 ID を指定された場合に「typo / 古い ID 名」 を即座に検出するため、 unknown は load 時 error にする。
func AllFeatures() []Feature {
	return []Feature{
		FeatureVendorSync, FeatureVendorReadonly, FeatureReadOnly,
		FeatureGitignore, FeatureClaudePlugins, FeatureTriggerWorkflow,
		FeatureClaudeRules, FeatureClaudeSkills,
		FeatureGolangciLint, FeatureMarkdownLint, FeatureShellcheck,
		FeatureDevbox, FeatureVscode, FeatureADR,
		FeatureEmbedClaudeMd, FeatureEmbedEnvrc, FeatureEmbedYamllint,
	}
}

// featureForInstall は install path と PlanKind から該当 Feature を導出する。 unknown の path は空文字を返す (= skip 対象判定で常に「無視されない」 扱い)。
func featureForInstall(installPath string, kind PlanKind) Feature {
	p := filepath.ToSlash(installPath)
	switch kind {
	case PlanKindTriggerWorkflow:
		return FeatureTriggerWorkflow
	case PlanKindEmbed:
		return featureForEmbed(p)
	case PlanKindInstallCopy:
		return featureForInstallCopy(p)
	case PlanKindUnknown:
		return ""
	}
	return ""
}

func featureForEmbed(installPath string) Feature {
	switch installPath {
	case "CLAUDE.md":
		return FeatureEmbedClaudeMd
	case ".envrc":
		return FeatureEmbedEnvrc
	case ".yamllint":
		return FeatureEmbedYamllint
	}
	return ""
}

func featureForInstallCopy(installPath string) Feature {
	switch installPath {
	case ".golangci.yaml":
		return FeatureGolangciLint
	case ".markdownlint-cli2.yaml":
		return FeatureMarkdownLint
	case ".shellcheckrc":
		return FeatureShellcheck
	case "devbox.json":
		return FeatureDevbox
	}
	switch {
	case strings.HasPrefix(installPath, ".vscode/"):
		return FeatureVscode
	case strings.HasPrefix(installPath, ".claude/rules/"):
		return FeatureClaudeRules
	case strings.HasPrefix(installPath, ".claude/skills/"):
		return FeatureClaudeSkills
	case strings.HasPrefix(installPath, "docs/adr/"):
		return FeatureADR
	}
	return ""
}
