// Package version は release 経路の semver 算出共通ロジック (FLM_FEA_0002 §版番号)。 前回 tag の取得・spec の flat map diff から bump kind を決める処理は tool / lib 系で共通化できるため本 package に集約する。 系統別の差異 (flatten 関数 / tag prefix / MAJOR 自動 release 可否) は caller が引数で渡す。
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wakuwaku3/flame/cli/internal/release/ghapi"
	"github.com/wakuwaku3/flame/lib/ex"
)

// Bump は spec diff から決まる semver 増分種別。
type Bump string

const (
	BumpInitial Bump = "initial"
	BumpMajor   Bump = "major"
	BumpMinor   Bump = "minor"
	BumpPatch   Bump = "patch"
)

// Plan は次バージョン算出結果。 Prior が空文字なら初版 (Bump=initial / Next=1.0.0)。
type Plan struct {
	Prior string
	Next  string
	Bump  Bump
}

// FlattenFunc は spec JSON を「key → value 文字列」 の flat map に変換する。 系統別 (tool: subcommand/flag / lib: package::kind::name) で異なるため caller が渡す。
type FlattenFunc func(specJSON []byte) (map[string]string, error)

// Input は Compute の入力。 ForbidMajor=true (library 系) のとき MAJOR 判定で error を返す。
type Input struct {
	GH               ghapi.Client
	Flatten          FlattenFunc
	Repo             string
	TagPrefix        string
	NewSpecPath      string
	WorkDir          string
	PriorTagOverride string
	SpecAssetName    string
	ForbidMajor      bool
}

// Compute は前回 release 取得 → spec diff → bump kind 判定 → 次バージョン算出を行う。 warn は warn ログの出力先 (前回 release の spec asset 不在等の non-fatal な逸脱経路で使用)。 in は Input を pointer で受ける (gocritic hugeParam: 128B 超は pointer 渡し)。
func Compute(ctx context.Context, warn io.Writer, in *Input) (*Plan, error) {
	prior, err := resolvePrior(ctx, in)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	if prior == "" {
		return &Plan{Prior: "", Next: "1.0.0", Bump: BumpInitial}, nil
	}
	if !semverCorePattern.MatchString(prior) {
		return nil, ex.Errorf("prior version '%s' is not canonical MAJOR.MINOR.PATCH semver", prior)
	}

	bump := BumpPatch
	oldSpecPath := filepath.Join(in.WorkDir, "old-spec.json")
	if downloadErr := in.GH.ReleaseDownload(ctx, in.TagPrefix+prior, in.SpecAssetName, oldSpecPath); downloadErr != nil {
		fmt.Fprintf(warn, "warn: prior release %s%s has no %s asset; defaulting to 'patch' bump\n", in.TagPrefix, prior, in.SpecAssetName)
	} else {
		oldSpec, readErr := os.ReadFile(oldSpecPath) //nolint:gosec // G304: oldSpecPath は WorkDir 配下の caller 制御下の path で、 直前に gh release download で書き出した spec を読み戻す経路。
		if readErr != nil {
			return nil, ex.Wrap(readErr)
		}
		newSpec, readErr := os.ReadFile(in.NewSpecPath) //nolint:gosec // G304: NewSpecPath は CLI 引数経由で caller (release-app/lib subcommand) が build 済 spec を書いた path で、 spec diff の入力として読み込むのが endpoint 責務。
		if readErr != nil {
			return nil, ex.Wrap(readErr)
		}
		oldMap, flattenErr := in.Flatten(oldSpec)
		if flattenErr != nil {
			return nil, ex.Wrap(flattenErr)
		}
		newMap, flattenErr := in.Flatten(newSpec)
		if flattenErr != nil {
			return nil, ex.Wrap(flattenErr)
		}
		bump = diffBump(oldMap, newMap)
	}

	if in.ForbidMajor && bump == BumpMajor {
		return nil, ex.Errorf("MAJOR bump detected; library MAJOR auto-release is not supported (FLM_FEA_0002 §版番号)")
	}
	next, err := bumpVersion(prior, bump)
	if err != nil {
		return nil, ex.Wrap(err)
	}
	return &Plan{Prior: prior, Next: next, Bump: bump}, nil
}

func resolvePrior(ctx context.Context, in *Input) (string, error) {
	if in.PriorTagOverride != "" {
		return strings.TrimPrefix(in.PriorTagOverride, in.TagPrefix), nil
	}
	listJSON, err := in.GH.API(ctx, fmt.Sprintf("/repos/%s/releases?per_page=100", in.Repo))
	if err != nil {
		// shell 版 `gh api ... 2>/dev/null` 失敗時の `||` 経路と同じく、 release 一覧取得失敗を「prior 不在 = 初版扱い」 にせず、 prior 空文字 (= 初版扱いにせず Compute 側で「prior == ""」 を初版に解釈) を返す。 ただし shell 版 release-app.sh は同経路で「release 0 件」 と区別したいときは exit するが、 Go 版は warn に倒さず空文字返しで callers に判定を委ねる (nilerr lint は意図的)。
		return "", nil //nolint:nilerr // shell 版の `||` 経路と同等の素通し挙動。
	}
	return latestVersionFromReleaseList(listJSON, in.TagPrefix)
}

// releaseEntry は gh release list の entry のうち本 package が消費する tag_name のみを抜き出す最小型。
type releaseEntry struct {
	TagName string `json:"tag_name"`
}

// latestVersionFromReleaseList は gh CLI の `--paginate` が複数 page を JSON array として並べる出力 (= concatenated arrays) を順次読み、 tag_name から prefix を剥がした semver core を集めて最大値を返す。 page 区切りの `[...][...]` を受けるため json.Decoder で stream 解析する。
func latestVersionFromReleaseList(data []byte, tagPrefix string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	var versions []string
	for dec.More() {
		var page []releaseEntry
		if err := dec.Decode(&page); err != nil {
			return "", ex.Wrap(err)
		}
		for _, r := range page {
			if rest, ok := strings.CutPrefix(r.TagName, tagPrefix); ok {
				versions = append(versions, rest)
			}
		}
	}
	if len(versions) == 0 {
		return "", nil
	}
	sort.Slice(versions, func(i, j int) bool { return semverLess(versions[i], versions[j]) })
	return versions[len(versions)-1], nil
}

func semverLess(a, b string) bool {
	ap := splitSemver(a)
	bp := splitSemver(b)
	for i := range semverParts {
		if ap[i] != bp[i] {
			return ap[i] < bp[i]
		}
	}
	return false
}

func splitSemver(v string) [semverParts]int {
	parts := strings.SplitN(v, ".", semverParts)
	var out [semverParts]int
	for i, p := range parts {
		if i >= semverParts {
			break
		}
		n, _ := strconv.Atoi(p)
		out[i] = n //nolint:gosec // G602: ループ前に `i >= semverParts` で out 範囲超を弾いているため、 配列固定サイズ semverParts 内に収まることが保証される (gosec の static 解析が読み取れない不変条件)。
	}
	return out
}

const semverParts = 3

var semverCorePattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func diffBump(oldMap, newMap map[string]string) Bump {
	for k, v := range oldMap {
		if nv, ok := newMap[k]; ok && nv != v {
			return BumpMajor
		}
	}
	for k := range newMap {
		if _, ok := oldMap[k]; !ok {
			return BumpMinor
		}
	}
	return BumpPatch
}

func bumpVersion(prior string, kind Bump) (string, error) {
	parts := strings.Split(prior, ".")
	if len(parts) != semverParts {
		return "", ex.Errorf("prior version '%s' must be MAJOR.MINOR.PATCH", prior)
	}
	nums := make([]int, semverParts)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return "", ex.Wrapf(err, "parse semver component %s", p)
		}
		nums[i] = n
	}
	switch kind {
	case BumpMajor:
		nums[0]++
		nums[1] = 0
		nums[2] = 0
	case BumpMinor:
		nums[1]++
		nums[2] = 0
	case BumpPatch:
		nums[2]++
	case BumpInitial:
		return "1.0.0", nil
	default:
		return "", ex.Errorf("unknown bump kind: %s", kind)
	}
	return fmt.Sprintf("%d.%d.%d", nums[0], nums[1], nums[2]), nil
}
