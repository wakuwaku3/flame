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
		isSelf    bool
		wantSkip  bool
	}{
		{name: "devbox.lock は常に skip", vendorRel: "vendor/flame/devbox.lock", isSelf: false, wantSkip: true},
		{name: ".github/workflows/tests/ は常に skip", vendorRel: "vendor/flame/.github/workflows/tests/x.sh", isSelf: false, wantSkip: true},
		{name: "devbox/init.sh は vendor 直接参照のため skip", vendorRel: "vendor/flame/devbox/init.sh", isSelf: false, wantSkip: true},
		{name: "self mode で docs/adr/ は skip (stub 経路)", vendorRel: "vendor/flame/docs/adr/general/X.md", isSelf: true, wantSkip: true},
		{name: "self mode で .claude/rules/ は skip (stub 経路)", vendorRel: "vendor/flame/.claude/rules/x.md", isSelf: true, wantSkip: true},
		{name: "downstream mode で docs/adr/ は install copy", vendorRel: "vendor/flame/docs/adr/general/X.md", isSelf: false, wantSkip: false},
		{name: "downstream mode で .claude/rules/ は install copy", vendorRel: "vendor/flame/.claude/rules/x.md", isSelf: false, wantSkip: false},
		{name: "通常の lint config は skip しない", vendorRel: "vendor/flame/.golangci.yaml", isSelf: true, wantSkip: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := skipVendorPath(tc.vendorRel, tc.isSelf)
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
