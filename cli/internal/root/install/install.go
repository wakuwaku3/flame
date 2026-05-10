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

// Run は `flame install` の主処理。 repo root を入力に取り、 (1) vendor 同期 / (2) install copy 合成 / (3) embed snippet 配置 / (4) flame.lock 生成 / (5) chmod 444 / (6) .gitignore / (7) plugin 登録 を順序実行する。
func Run(ctx context.Context, repoRoot string, stdout, stderr io.Writer) error {
	m, err := LoadManifest(ctx, repoRoot)
	if err != nil {
		return err
	}
	prevLock, err := LoadLock(ctx, repoRoot)
	if err != nil {
		return err
	}
	if syncErr := SyncVendor(ctx, repoRoot, m); syncErr != nil {
		return syncErr
	}
	plan, err := BuildPlan(ctx, repoRoot, m)
	if err != nil {
		return err
	}
	overlayBy := indexOverlaysByInstall(prevLock)
	mergeArrayBy := indexMergeArrayByInstall(prevLock)
	newLock := &Lock{Files: nil, Embeds: nil}
	for _, item := range plan.Items {
		if execErr := executeItem(ctx, repoRoot, m, item, overlayBy, mergeArrayBy, newLock); execErr != nil {
			return execErr
		}
	}
	sortLockFiles(newLock)
	if err := WriteLock(ctx, repoRoot, newLock); err != nil {
		return err
	}
	if err := applyReadOnly(ctx, repoRoot, plan); err != nil {
		return err
	}
	if !m.SkipGitignore() {
		if err := applyGitignore(ctx, repoRoot); err != nil {
			return err
		}
	}
	if !m.SkipPluginInstall() {
		if err := applyPluginMarketplace(ctx, repoRoot, m.Source); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "flame install: %s\n", FormatStrategySummary(newLock))
	_ = stderr
	return nil
}

func executeItem(ctx context.Context, repoRoot string, m *Manifest, item PlanItem, overlayBy map[string]LockOverlay, mergeArrayBy map[string]MergeArrayStrategy, newLock *Lock) error {
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
		return executeInstallCopy(ctx, repoRoot, m, item, overlayBy, mergeArrayBy, newLock)
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

func executeInstallCopy(ctx context.Context, repoRoot string, m *Manifest, item PlanItem, overlayBy map[string]LockOverlay, mergeArrayBy map[string]MergeArrayStrategy, newLock *Lock) error {
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
	arrayStrategy := mergeArrayBy[filepath.ToSlash(item.InstallPath)]
	merged, err := Merge(&MergeInput{
		VendorContent:  vendorBytes,
		OverlayContent: overlayBytes,
		InstallPath:    item.InstallPath,
		Strategy:       item.Merge,
		ArrayStrategy:  arrayStrategy,
	})
	if err != nil {
		return err
	}
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
		MergeArray:    arrayStrategy,
	}
	switch {
	case overlayPath != "":
		entry.Overlay = &LockOverlay{Path: overlayPath, Content: string(overlayBytes)}
	default:
		if existing, ok := overlayBy[filepath.ToSlash(item.InstallPath)]; ok {
			entry.Overlay = &LockOverlay{Path: existing.Path, Content: existing.Content}
		}
	}
	newLock.Files = append(newLock.Files, entry)
	return nil
}

// readOverlay は install path に対応する `<basename>.flame-overlay.<ext>` を読み込む。 file が存在しなければ (nil, "", nil) を返す。
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

// overlayPathFor は install path から副ファイル overlay の path を導く。 拡張子のあるファイルは `<basename-without-ext>.flame-overlay.<ext>`、 無いファイルは `<basename>.flame-overlay`。
func overlayPathFor(installPath string) string {
	dir := filepath.Dir(installPath)
	base := filepath.Base(installPath)
	ext := filepath.Ext(base)
	if ext == "" {
		return filepath.Join(dir, base+".flame-overlay")
	}
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+".flame-overlay"+ext)
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

func indexOverlaysByInstall(prev *Lock) map[string]LockOverlay {
	out := map[string]LockOverlay{}
	for _, f := range prev.Files {
		if f.Overlay != nil {
			out[f.Install] = *f.Overlay
		}
	}
	return out
}

func indexMergeArrayByInstall(prev *Lock) map[string]MergeArrayStrategy {
	out := map[string]MergeArrayStrategy{}
	for _, f := range prev.Files {
		if f.MergeArray != "" {
			out[f.Install] = f.MergeArray
		}
	}
	return out
}

func sortLockFiles(lock *Lock) {
	sort.SliceStable(lock.Files, func(i, j int) bool {
		return lock.Files[i].Install < lock.Files[j].Install
	})
}

// applyReadOnly は install copy / trg__ scaffold の install 先を chmod 444 で確定させる (FLM_FEA_0003 §install 先の read-only 強制)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func applyReadOnly(_ context.Context, repoRoot string, plan *Plan) error {
	for _, item := range plan.Items {
		if item.Kind == PlanKindEmbed {
			continue
		}
		path := filepath.Join(repoRoot, item.InstallPath)
		if chmodErr := os.Chmod(path, readOnlyPerm); chmodErr != nil && !os.IsPermission(chmodErr) {
			return ex.Wrapf(chmodErr, "chmod 444: %s", path)
		}
	}
	return nil
}
