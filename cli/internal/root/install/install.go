// Package install は `flame install` subcommand を実装する ([FLM_FEA_0003](../../../../docs/adr/feature/FLM_FEA_0003__harness.md))。 flame.yaml manifest を読み、 vendor SoT (`vendor/flame/`) を install 先 (= repo root) に同期し flame.lock を生成・更新する。 利用側 / flame self の両方で同じ subcommand 経路を共有する。
package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wakuwaku3/flame/lib/ex"
)

func Run(ctx context.Context, repoRoot string, stdout, stderr io.Writer) error {
	m, err := LoadManifest(ctx, repoRoot)
	if err != nil {
		return err
	}
	prevLock, err := LoadLock(ctx, repoRoot)
	if err != nil {
		return err
	}
	if syncErr := SyncVendor(ctx, repoRoot, m, prevLock); syncErr != nil {
		return syncErr
	}
	plan, err := BuildPlan(ctx, repoRoot, m)
	if err != nil {
		return err
	}
	prevByInstall := indexLockFilesByInstall(prevLock)
	newLock := &Lock{Installed: nil, Files: nil, Embeds: nil}
	for _, item := range plan.Items {
		if execErr := executeItem(ctx, repoRoot, m, item, prevByInstall, newLock); execErr != nil {
			return execErr
		}
	}
	sortLockFiles(newLock)
	installed, instErr := buildInstalledRecord(ctx, repoRoot, m)
	if instErr != nil {
		return instErr
	}
	newLock.Installed = installed
	if err := WriteLock(ctx, repoRoot, newLock); err != nil {
		return err
	}
	if !m.IsIgnored(FeatureReadOnly) {
		if err := applyReadOnly(ctx, repoRoot, m, plan); err != nil {
			return err
		}
	}
	if !m.IsIgnored(FeatureGitignore) {
		if err := applyGitignore(ctx, repoRoot); err != nil {
			return err
		}
	}
	if !m.IsIgnored(FeatureClaudePlugins) {
		if err := applyPluginMarketplace(ctx, repoRoot, m.Source); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "flame install: %s\n", FormatStrategySummary(newLock))
	_ = stderr
	return nil
}

func executeItem(ctx context.Context, repoRoot string, m *Manifest, item PlanItem, prevByInstall map[string]LockFile, newLock *Lock) error {
	if feat := featureForInstall(item.InstallPath, item.Kind); m.IsIgnored(feat) {
		return nil
	}
	switch item.Kind {
	case PlanKindEmbed:
		entry, err := applyEmbed(ctx, repoRoot, item)
		if err != nil {
			return err
		}
		newLock.Embeds = append(newLock.Embeds, entry)
		return nil
	case PlanKindTriggerWorkflow:
		return executeTriggerWorkflow(ctx, repoRoot, m, item)
	case PlanKindInstallCopy:
		return executeInstallCopy(ctx, repoRoot, m, item, prevByInstall, newLock)
	case PlanKindUnknown:
		return ex.Errorf("unknown plan kind for %s", item.VendorPath)
	default:
		return ex.Errorf("unsupported plan kind: %v (path=%s)", item.Kind, item.VendorPath)
	}
}

// executeTriggerWorkflow は flame-trg__*.yaml を 1 回限りの bootstrap として install する (FLM_FEA_0003 §`flame.lock` 整合性検査対象外)。 既に install 先に file が存在する場合は no-op (= 利用側がカスタマイズしたものをそのまま尊重する)。
func executeTriggerWorkflow(ctx context.Context, repoRoot string, m *Manifest, item PlanItem) error {
	installAbs := filepath.Join(repoRoot, item.InstallPath)
	if _, statErr := os.Stat(installAbs); statErr == nil {
		return nil
	}
	vendorAbs := filepath.Join(repoRoot, item.VendorPath)
	vendorBytes, err := os.ReadFile(vendorAbs)
	if err != nil {
		return ex.Wrapf(err, "read vendor: %s", vendorAbs)
	}
	pinned, err := pinUsesRef(vendorBytes, m.Source, m.Version)
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(installAbs), dirPerm); mkErr != nil {
		return ex.Wrapf(mkErr, "mkdir parent: %s", installAbs)
	}
	return writeWritable(ctx, installAbs, pinned)
}

func executeInstallCopy(ctx context.Context, repoRoot string, m *Manifest, item PlanItem, prevByInstall map[string]LockFile, newLock *Lock) error {
	vendorAbs := filepath.Join(repoRoot, item.VendorPath)
	installAbs := filepath.Join(repoRoot, item.InstallPath)
	vendorBytes, err := os.ReadFile(vendorAbs)
	if err != nil {
		return ex.Wrapf(err, "read vendor: %s", vendorAbs)
	}
	overlayBytes, overlayPath, err := readOverlay(ctx, repoRoot, item.InstallPath)
	if err != nil {
		return err
	}
	prevEntry := prevByInstall[filepath.ToSlash(item.InstallPath)]
	out, mergeErr := Merge3Way(&Merge3WayInput{
		Strategy:     item.Merge,
		InstallPath:  item.InstallPath,
		BaseContent:  []byte(prevEntry.VendorContent),
		TheirContent: vendorBytes,
		OurContent:   overlayBytes,
	})
	if mergeErr != nil {
		return mergeErr
	}
	if len(out.Conflicts) > 0 {
		if writeErr := writeConflictMarker(repoRoot, item.InstallPath, overlayBytes, vendorBytes, out.Conflicts); writeErr != nil {
			return writeErr
		}
		return formatConflictError(item.InstallPath, out.Conflicts)
	}
	merged := out.Content
	if strings.HasPrefix(filepath.ToSlash(item.InstallPath), githubWorkflowsDir+"/") {
		merged, err = pinUsesRef(merged, m.Source, m.Version)
		if err != nil {
			return err
		}
	}
	if mkErr := os.MkdirAll(filepath.Dir(installAbs), dirPerm); mkErr != nil {
		return ex.Wrapf(mkErr, "mkdir parent: %s", installAbs)
	}
	if writeErr := writeWritable(ctx, installAbs, merged); writeErr != nil {
		return writeErr
	}
	entry := LockFile{
		Install:       filepath.ToSlash(item.InstallPath),
		Vendor:        filepath.ToSlash(item.VendorPath),
		Merge:         item.Merge,
		Content:       string(merged),
		VendorContent: string(vendorBytes),
		Overlay:       nil,
	}
	switch {
	case overlayPath != "":
		entry.Overlay = &LockOverlay{Path: overlayPath, Content: string(overlayBytes)}
	default:
		if prevEntry.Overlay != nil {
			entry.Overlay = &LockOverlay{Path: prevEntry.Overlay.Path, Content: prevEntry.Overlay.Content}
		}
	}
	newLock.Files = append(newLock.Files, entry)
	return nil
}

func readOverlay(_ context.Context, repoRoot, installPath string) (content []byte, overlayPath string, err error) {
	overlayRel := overlayPathFor(installPath)
	overlayAbs := filepath.Join(repoRoot, overlayRel)
	data, readErr := os.ReadFile(overlayAbs)
	if os.IsNotExist(readErr) {
		return nil, "", nil
	}
	if readErr != nil {
		return nil, "", ex.Wrapf(readErr, "read overlay: %s", overlayAbs)
	}
	return data, filepath.ToSlash(overlayRel), nil
}

// overlayPathFor は install path から副ファイル overlay の path を導く。 拡張子のあるファイルは `<basename-without-ext>.flame-overlay.<ext>`、 無いファイルは `<basename>.flame-overlay` (FLM_FEA_0003 §副ファイル overlay 機構)。 hidden file (= `.shellcheckrc` 等の dot 始まりかつ単一 dot) は no-ext 扱い (Go の `filepath.Ext` は hidden file の filename 全体を ext として返す挙動のため、 ここで個別判定する)。
func overlayPathFor(installPath string) string {
	dir := filepath.Dir(installPath)
	base := filepath.Base(installPath)
	ext := overlayExt(base)
	if ext == "" {
		return filepath.Join(dir, base+".flame-overlay")
	}
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+".flame-overlay"+ext)
}

func overlayExt(base string) string {
	if strings.HasPrefix(base, ".") && strings.Count(base, ".") == 1 {
		return ""
	}
	return filepath.Ext(base)
}

func writeWritable(_ context.Context, path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		if chmodErr := os.Chmod(path, filePerm); chmodErr != nil {
			return ex.Wrapf(chmodErr, "chmod writable: %s", path)
		}
	}
	if writeErr := os.WriteFile(path, content, filePerm); writeErr != nil {
		return ex.Wrapf(writeErr, "write %s", path)
	}
	return nil
}

func indexLockFilesByInstall(prev *Lock) map[string]LockFile {
	out := map[string]LockFile{}
	for _, f := range prev.Files {
		out[f.Install] = f
	}
	return out
}

// writeConflictMarker は 3-way merge で conflict が検出された場合に overlay 副ファイルへ git 形式の conflict marker を書き戻す (FLM_FEA_0003 §副ファイル overlay 機構 §conflict 発生時の挙動)。 ADR は inline marker (= 構造化 3-way なら該当 mapping value / sequence 要素レベルに埋め込み) を要求するが、 本実装は first-cut として overlay 全体を OURS / THEIRS で wrap する簡易形式を採用する。 結果として overlay は次回 manifest load 時に parse 失敗で fail する (= 利用者に解決を促す)。 inline marker 化は follow-up とする。
func writeConflictMarker(repoRoot, installPath string, overlayContent, vendorContent []byte, conflicts []MergeConflict) error {
	overlayRel := overlayPathFor(installPath)
	overlayAbs := filepath.Join(repoRoot, overlayRel)
	var b strings.Builder
	fmt.Fprintf(&b, "# flame install: 3-way merge conflict (install=%s)\n", installPath)
	for _, c := range conflicts {
		fmt.Fprintf(&b, "#   - path=%s: %s\n", c.Path, c.Description)
	}
	b.WriteString("# Resolve by editing this file (remove markers and apply intended values), then re-run `flame install`.\n")
	b.WriteString("<<<<<<< OURS (current overlay)\n")
	b.Write(overlayContent)
	if len(overlayContent) > 0 && overlayContent[len(overlayContent)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("=======\n")
	b.Write(vendorContent)
	if len(vendorContent) > 0 && vendorContent[len(vendorContent)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(">>>>>>> THEIRS (current vendor)\n")
	if mkErr := os.MkdirAll(filepath.Dir(overlayAbs), dirPerm); mkErr != nil {
		return ex.Wrapf(mkErr, "mkdir overlay parent: %s", overlayAbs)
	}
	if writeErr := os.WriteFile(overlayAbs, []byte(b.String()), filePerm); writeErr != nil {
		return ex.Wrapf(writeErr, "write conflict marker overlay: %s", overlayAbs)
	}
	return nil
}

func formatConflictError(installPath string, conflicts []MergeConflict) error {
	var b strings.Builder
	fmt.Fprintf(&b, "merge conflict at %s (overlay vs vendor): %d conflict(s)\n", installPath, len(conflicts))
	for _, c := range conflicts {
		fmt.Fprintf(&b, "  - path=%s: %s\n", c.Path, c.Description)
	}
	b.WriteString("resolve by editing the overlay file (`*.flame-overlay.*`), then re-run `flame install`")
	return ex.Errorf("%s", b.String())
}

func sortLockFiles(lock *Lock) {
	sort.SliceStable(lock.Files, func(i, j int) bool {
		return lock.Files[i].Install < lock.Files[j].Install
	})
}

// buildInstalledRecord は flame.lock.installed セクションの内容 (= 前回 install 実行時の source / version / vendor tree hash) を組み立てる (FLM_FEA_0003 §flame.lock の installed)。 self mode では tree_hash を空に保つ (working tree が常時変動するため CI が壊れる) という ADR の規定に従う。
func buildInstalledRecord(ctx context.Context, repoRoot string, m *Manifest) (*LockInstalled, error) {
	out := &LockInstalled{Source: m.Source, Version: m.Version, TreeHash: ""}
	if m.IsSelf() {
		return out, nil
	}
	hash, err := ComputeVendorTreeHash(ctx, filepath.Join(repoRoot, VendorRoot))
	if err != nil {
		return nil, err
	}
	out.TreeHash = hash
	return out, nil
}

// applyReadOnly は install copy / trg__ scaffold の install 先を chmod 444 で確定させる (FLM_FEA_0003 §install 先の read-only 強制)。 manifest で個別 Feature が ignore されている entry は元々 install されていないため対象外。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func applyReadOnly(_ context.Context, repoRoot string, m *Manifest, plan *Plan) error {
	for _, item := range plan.Items {
		if item.Kind == PlanKindEmbed {
			continue
		}
		if m.IsIgnored(featureForInstall(item.InstallPath, item.Kind)) {
			continue
		}
		path := filepath.Join(repoRoot, item.InstallPath)
		if chmodErr := os.Chmod(path, readOnlyPerm); chmodErr != nil && !os.IsPermission(chmodErr) {
			return ex.Wrapf(chmodErr, "chmod 444: %s", path)
		}
	}
	return nil
}
