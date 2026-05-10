package install

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeatureForInstall(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		installPath string
		kind        PlanKind
		want        Feature
	}{
		{name: ".golangci.yaml は golangci-lint", installPath: ".golangci.yaml", kind: PlanKindInstallCopy, want: FeatureGolangciLint},
		{name: ".markdownlint-cli2.yaml は markdown-lint", installPath: ".markdownlint-cli2.yaml", kind: PlanKindInstallCopy, want: FeatureMarkdownLint},
		{name: ".shellcheckrc は shellcheck", installPath: ".shellcheckrc", kind: PlanKindInstallCopy, want: FeatureShellcheck},
		{name: "devbox.json は devbox", installPath: "devbox.json", kind: PlanKindInstallCopy, want: FeatureDevbox},
		{name: ".vscode/settings.json は vscode", installPath: ".vscode/settings.json", kind: PlanKindInstallCopy, want: FeatureVscode},
		{name: ".claude/rules/x.md は claude/rules", installPath: ".claude/rules/x.md", kind: PlanKindInstallCopy, want: FeatureClaudeRules},
		{name: ".claude/skills/x.md は claude/skills", installPath: ".claude/skills/x.md", kind: PlanKindInstallCopy, want: FeatureClaudeSkills},
		{name: "docs/adr/x.md は adr", installPath: "docs/adr/general/x.md", kind: PlanKindInstallCopy, want: FeatureADR},
		{name: "trigger workflow は trigger-workflow", installPath: ".github/workflows/flame-trg__push__main.yaml", kind: PlanKindTriggerWorkflow, want: FeatureTriggerWorkflow},
		{name: "CLAUDE.md embed は embed/claude-md", installPath: "CLAUDE.md", kind: PlanKindEmbed, want: FeatureEmbedClaudeMd},
		{name: ".envrc embed は embed/envrc", installPath: ".envrc", kind: PlanKindEmbed, want: FeatureEmbedEnvrc},
		{name: ".yamllint embed は embed/yamllint", installPath: ".yamllint", kind: PlanKindEmbed, want: FeatureEmbedYamllint},
		{name: "未知 install copy は空文字", installPath: "unknown.txt", kind: PlanKindInstallCopy, want: ""},
		{name: "未知 embed は空文字", installPath: "unknown", kind: PlanKindEmbed, want: ""},
		{name: "PlanKindUnknown は空文字", installPath: ".golangci.yaml", kind: PlanKindUnknown, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := featureForInstall(tc.installPath, tc.kind)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAllFeatures_NoDuplicates(t *testing.T) {
	t.Parallel()
	all := AllFeatures()
	seen := map[Feature]struct{}{}
	for _, f := range all {
		_, dup := seen[f]
		assert.Falsef(t, dup, "duplicate feature: %s", f)
		seen[f] = struct{}{}
	}
}

func TestAllFeatures_ContainsCoreSteps(t *testing.T) {
	t.Parallel()
	all := AllFeatures()
	names := make([]string, len(all))
	for i, f := range all {
		names[i] = string(f)
	}
	sort.Strings(names)
	for _, required := range []string{"gitignore", "claude/plugins", "vendor-sync", "vendor-readonly", "read-only", "trigger-workflow"} {
		idx := sort.SearchStrings(names, required)
		assert.Truef(t, idx < len(names) && names[idx] == required, "missing required feature: %s", required)
	}
}
