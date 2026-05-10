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

// SyncVendor は manifest の `flame.source` / `version` に基づき vendor/flame/ を最新化する。 version が `self` の場合は当該 repo の working tree が SoT のため fetch を行わず、 vendor/flame/ の存在のみ確認する。 それ以外は vendor/flame/ が既に存在する場合は idempotent install として fetch を skip し、 存在しない場合のみ git clone で source repo を temp dir に取得し vendor/flame/ subdirectory を repo root の vendor/flame/ に sync する (FLM_FEA_0003 §チャネル C: vendor sync は version 単位の fetch + repo-local idempotent 適用の 2 段階運用)。 clone 直後に vendor 配下を chmod 444 (file) / 555 (dir) で readonly 化し、 利用者が誤って vendor を直接編集する経路を OS 層で塞ぐ。 manifest で `vendor-sync` または `vendor-readonly` が ignore されている場合は該当工程を skip する。 version 変更時の強制再 fetch は Step 5 で実装。
func SyncVendor(ctx context.Context, repoRoot string, m *Manifest) error {
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
		return nil
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
