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

// SyncVendor は manifest の `flame.harness.source` / `version` に基づき vendor/flame/ を最新化する。 version が `self` の場合は当該 repo の working tree が SoT のため fetch を行わず、 vendor/flame/ の存在のみ確認する。 それ以外は vendor/flame/ が既に存在する場合は idempotent install として fetch を skip し、 存在しない場合のみ git clone で source repo を temp dir に取得し vendor/flame/ subdirectory を repo root の vendor/flame/ に sync する (FLM_FEA_0003 §チャネル C: vendor sync は version 単位の fetch + repo-local idempotent 適用の 2 段階運用)。 version 変更時の強制再 fetch は将来 `--refresh` flag で対応する。
func SyncVendor(ctx context.Context, repoRoot string, m *Manifest) error {
	vendorAbs := filepath.Join(repoRoot, VendorRoot)
	if m.IsSelf() {
		if _, err := os.Stat(vendorAbs); err != nil {
			return ex.Wrapf(err, "vendor SoT not found in self mode (working tree must contain %s): %s", VendorRoot, vendorAbs)
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
	return copyTree(ctx, srcVendor, vendorAbs)
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
