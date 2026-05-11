package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveInstallPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		vendorRel   string
		wantInstall string
		wantKind    PlanKind
	}{
		{
			name:        "lint config は repo root 直下に install copy",
			vendorRel:   "vendor/flame/.golangci.yaml",
			wantInstall: ".golangci.yaml",
			wantKind:    PlanKindInstallCopy,
		},
		{
			name:        "GitHub Actions trigger workflow は flame- prefix で trigger workflow 扱い",
			vendorRel:   "vendor/flame/.github/workflows/trg__push__main.yaml",
			wantInstall: ".github/workflows/flame-trg__push__main.yaml",
			wantKind:    PlanKindTriggerWorkflow,
		},
		{
			name:        ".vscode/settings.json は install copy",
			vendorRel:   "vendor/flame/.vscode/settings.json",
			wantInstall: ".vscode/settings.json",
			wantKind:    PlanKindInstallCopy,
		},
		{
			name:        "trg__ 以外の .github/workflows は unknown",
			vendorRel:   "vendor/flame/.github/workflows/wf__check.yaml",
			wantInstall: "",
			wantKind:    PlanKindUnknown,
		},
		{
			name:        "vendor 配下でない path は unknown",
			vendorRel:   "other/path.txt",
			wantInstall: "",
			wantKind:    PlanKindUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			install, kind := resolveInstallPath(tc.vendorRel)
			assert.Equal(t, tc.wantInstall, install)
			assert.Equal(t, tc.wantKind, kind)
		})
	}
}

func TestSkipVendorPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		vendorRel string
		wantSkip  bool
	}{
		{name: "devbox.lock は skip", vendorRel: "vendor/flame/devbox.lock", wantSkip: true},
		{name: ".github/workflows/tests/ は skip", vendorRel: "vendor/flame/.github/workflows/tests/x.sh", wantSkip: true},
		{name: "devbox/init.sh は vendor 直接参照のため skip", vendorRel: "vendor/flame/devbox/init.sh", wantSkip: true},
		{name: "schemas/ は skip (vendor SoT 直接参照)", vendorRel: "vendor/flame/schemas/flame.yaml.schema.yaml", wantSkip: true},
		{name: "docs/adr/ は install copy (skip しない、 必要なら ignore: [adr] で制御)", vendorRel: "vendor/flame/docs/adr/general/X.md", wantSkip: false},
		{name: ".claude/rules/ は install copy (skip しない、 必要なら ignore: [claude/rules] で制御)", vendorRel: "vendor/flame/.claude/rules/x.md", wantSkip: false},
		{name: "通常の lint config は skip しない", vendorRel: "vendor/flame/.golangci.yaml", wantSkip: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := skipVendorPath(tc.vendorRel)
			assert.Equal(t, tc.wantSkip, got)
		})
	}
}

func TestDefaultMergeStrategy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		installPath string
		want        MergeStrategy
	}{
		{name: ".yaml は deep", installPath: "config.yaml", want: MergeDeep},
		{name: ".yml は deep", installPath: "config.yml", want: MergeDeep},
		{name: ".json は deep", installPath: ".vscode/settings.json", want: MergeDeep},
		{name: ".sh は append", installPath: "init.sh", want: MergeAppend},
		{name: ".md は append", installPath: "doc.md", want: MergeAppend},
		{name: "拡張子なしは append", installPath: ".shellcheckrc", want: MergeAppend},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := defaultMergeStrategy(tc.installPath)
			assert.Equal(t, tc.want, got)
		})
	}
}
