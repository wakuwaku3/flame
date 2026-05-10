package install

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/ex"
)

// SyncVendor は manifest の `flame.source` / `version` に基づき vendor/flame/ を最新化する。 version が `self` の場合は当該 repo の working tree が SoT のため fetch を行わず、 vendor/flame/ の存在のみ確認する。 それ以外は前回 install 時の installed.{source, version} と manifest の現在値を比較し、 不一致なら既存 vendor を削除して再 clone する。 一致または初回 install の場合は git clone で source repo を temp dir に取得し vendor/flame/ subdirectory を repo root の vendor/flame/ に sync する (FLM_FEA_0003 §チャネル C: vendor sync は version 単位の fetch + repo-local idempotent 適用)。 clone 直後に vendor 配下を chmod 444 (file) / 555 (dir) で readonly 化し、 利用者が誤って vendor を直接編集する経路を OS 層で塞ぐ。 manifest で `vendor-sync` または `vendor-readonly` が ignore されている場合は該当工程を skip する。
func SyncVendor(ctx context.Context, repoRoot string, m *Manifest, prev *Lock) error {
	vendorAbs := filepath.Join(repoRoot, VendorRoot)
	if m.IsSelf() {
		if _, err := os.Stat(vendorAbs); err != nil {
			return ex.Wrapf(err, "vendor SoT not found in self mode (working tree must contain %s): %s", VendorRoot, vendorAbs)
		}
		return nil
	}
	if m.IsIgnored(FeatureVendorSync) {
		if _, err := os.Stat(vendorAbs); err != nil {
			return ex.Wrapf(err, "vendor not found and `vendor-sync` is ignored (provide vendor manually): %s", vendorAbs)
		}
		return nil
	}
	if _, err := os.Stat(vendorAbs); err == nil {
		if !needsRefetch(prev, m) {
			return nil
		}
		if removeErr := removeVendorTree(ctx, vendorAbs); removeErr != nil {
			return removeErr
		}
	}
	tmpDir, err := os.MkdirTemp("", "flame-install-*")
	if err != nil {
		return ex.Wrap(err)
	}
	defer cleanupTempDir(tmpDir)
	if cloneErr := gitClone(ctx, m.Source, m.Version, tmpDir); cloneErr != nil {
		return cloneErr
	}
	srcVendor := filepath.Join(tmpDir, VendorRoot)
	if _, statErr := os.Stat(srcVendor); statErr != nil {
		return ex.Wrapf(statErr, "source repo does not contain %s at version %s", VendorRoot, m.Version)
	}
	if copyErr := copyTree(ctx, srcVendor, vendorAbs); copyErr != nil {
		return copyErr
	}
	if m.IsIgnored(FeatureVendorReadonly) {
		return nil
	}
	return makeVendorReadOnly(ctx, vendorAbs)
}

// needsRefetch は前回 install 時の installed セクションと現在の manifest を比較し、 source / version の少なくとも一方が変わっていれば true を返す。 prev または prev.Installed が nil の場合は「前回 install 状態が記録されていない (= 旧 schema または初回)」 とみなして false を返し、 既存 vendor をそのまま流用する (drift 検出は別工程に任せる)。
func needsRefetch(prev *Lock, m *Manifest) bool {
	if prev == nil || prev.Installed == nil {
		return false
	}
	return prev.Installed.Source != m.Source || prev.Installed.Version != m.Version
}

// removeVendorTree は readonly 化された vendor 配下を一旦 writable に戻してから削除する。 chmod を戻さずに RemoveAll すると EACCES で失敗するため、 戻し → 削除の 2 段階で行う。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func removeVendorTree(ctx context.Context, vendorRoot string) error {
	if err := makeVendorWritable(ctx, vendorRoot); err != nil {
		return err
	}
	return ex.Wrapf(os.RemoveAll(vendorRoot), "remove vendor: %s", vendorRoot)
}

// makeVendorReadOnly は vendor/flame 配下を walk して file は 444、 dir は 555 で readonly 化する (FLM_FEA_0003 §vendor の load 時 readonly)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func makeVendorReadOnly(_ context.Context, vendorRoot string) error {
	walkErr := filepath.WalkDir(vendorRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return ex.Wrap(err)
		}
		mode := readOnlyPerm
		if d.IsDir() {
			mode = readOnlyDirPerm
		}
		if chmodErr := os.Chmod(path, mode); chmodErr != nil {
			return ex.Wrapf(chmodErr, "chmod readonly: %s", path)
		}
		return nil
	})
	return ex.Wrap(walkErr)
}

// makeVendorWritable は vendor/flame 配下を walk して file は 644、 dir は 755 に戻す。 vendor 再 fetch (Step 5) や test cleanup で readonly を一時解除する経路で利用する。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func makeVendorWritable(_ context.Context, vendorRoot string) error {
	walkErr := filepath.WalkDir(vendorRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return ex.Wrap(err)
		}
		mode := filePerm
		if d.IsDir() {
			mode = dirPerm
		}
		if chmodErr := os.Chmod(path, mode); chmodErr != nil {
			return ex.Wrapf(chmodErr, "chmod writable: %s", path)
		}
		return nil
	})
	return ex.Wrap(walkErr)
}

func cleanupTempDir(dir string) {
	_ = os.RemoveAll(dir)
}

func gitClone(ctx context.Context, source, version, dst string) error {
	url := normalizeGitURL(source)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", version, url, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ex.Wrapf(err, "git clone %s@%s failed: %s", url, version, strings.TrimSpace(string(out)))
	}
	return nil
}

func normalizeGitURL(source string) string {
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "git@") {
		return source
	}
	return "https://" + source
}

const execMask fs.FileMode = 0o111

func copyTree(_ context.Context, src, dst string) error {
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ex.Wrap(walkErr)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return ex.Wrap(err)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return ex.Wrapf(os.MkdirAll(target, dirPerm), "mkdir %s", target)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ex.Wrapf(err, "read %s", path)
		}
		mode := filePerm
		info, infoErr := d.Info()
		if infoErr == nil && info.Mode().Perm()&execMask != 0 {
			mode = fsperm.Exec
		}
		if writeErr := os.WriteFile(target, data, mode); writeErr != nil {
			return ex.Wrapf(writeErr, "write %s", target)
		}
		return nil
	})
	return ex.Wrap(walkErr)
}
