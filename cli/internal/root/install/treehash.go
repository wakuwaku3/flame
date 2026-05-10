package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/wakuwaku3/flame/lib/ex"
)

// ComputeVendorTreeHash は vendor/flame 配下を walk し、 全 file の content hash と相対 path を順序固定で連結したものを sha256 で hash した値を返す (FLM_FEA_0003 §flame.lock の installed.tree_hash)。 git tree hash と等価ではないが、 git に依存せず再現可能で、 vendor 改変検知に十分な安定性を持つ。 戻り値の形式は "sha256:<hex>"。 vendorRoot が存在しない / 空 dir は error にせず "sha256:" + 空文字列 hash を返す (caller 側で意味を区別したい場合は独自に判定する)。 ctx は IO を含む関数 signature 規約 (FLM_APP_0007 §context 伝搬) に従い受け取るが本処理は同期 file IO のみ。
func ComputeVendorTreeHash(_ context.Context, vendorRoot string) (string, error) {
	type fileEntry struct {
		relPath string
		hash    [sha256.Size]byte
	}
	var entries []fileEntry
	walkErr := filepath.WalkDir(vendorRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return ex.Wrap(err)
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(vendorRoot, path)
		if relErr != nil {
			return ex.Wrap(relErr)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return ex.Wrapf(readErr, "read %s", path)
		}
		entries = append(entries, fileEntry{
			relPath: filepath.ToSlash(rel),
			hash:    sha256.Sum256(data),
		})
		return nil
	})
	if walkErr != nil {
		return "", ex.Wrap(walkErr)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].relPath < entries[j].relPath
	})
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s\x00%s\n", e.relPath, hex.EncodeToString(e.hash[:]))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
