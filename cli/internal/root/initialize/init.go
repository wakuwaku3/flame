package initialize

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wakuwaku3/flame/lib/ex"
)

// DefaultSource は flame.source の default 値。 flame harness の source 提供元 repo を固定値で記録する ([FLI_FEA_0002](../../../../docs/adr/feature/FLI_FEA_0002__flame_cli.md) §default 値の決定経路)。
const DefaultSource = "github.com/wakuwaku3/flame"

// PlaceholderVersion は当該 binary が dev build (= ldflags 未注入で `0.0.0-dev`) で release tag 形式に conform しない場合に flame.yaml に書き出す placeholder。 schema (^v\d+\.\d+\.\d+$) を満たすため利用者の手動編集前提でも install 起動できる。
const PlaceholderVersion = "v0.0.0"

const filePerm os.FileMode = 0o644

// semverPattern は flame.yaml.schema.yaml の version パターン (^v\d+\.\d+\.\d+$) に合わせた regex。 init 時の binary version 整形に使う。
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// RunOptions は init の入力。 BinaryVersion は当該 flame binary の version (root.Version) を受け取り、 default version 算出に使う。 Yes が true なら対話 skip + 全 default 採用 (`-y` 相当)。 SourceOverride / VersionOverride が空文字なら default + 対話、 非空なら当該 field の対話を skip して値を直接採用する (FLI_FEA_0002 §flame init による flame.yaml の初期生成)。
type RunOptions struct {
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	RepoRoot        string
	BinaryVersion   string
	SourceOverride  string
	VersionOverride string
	Yes             bool
}

// Run は cwd (= RepoRoot) 直下に flame.yaml を生成する。 既存ファイルがあれば上書きせず error 終了する。 opts は内部状態を一切持たない値オブジェクトだが、 hugeParam (>=80 bytes) 回避のため pointer で受け取る。
func Run(_ context.Context, opts *RunOptions) error {
	manifestPath := filepath.Join(opts.RepoRoot, "flame.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		return ex.Errorf("flame.yaml already exists at %s (remove it before re-running `flame init`)", manifestPath)
	} else if !os.IsNotExist(err) {
		return ex.Wrapf(err, "stat flame.yaml: %s", manifestPath)
	}

	defaultVersion := deriveDefaultVersion(opts.BinaryVersion)
	source, version, err := resolveValues(opts, defaultVersion)
	if err != nil {
		return err
	}

	content := renderManifest(source, version)
	if writeErr := os.WriteFile(manifestPath, []byte(content), filePerm); writeErr != nil {
		return ex.Wrapf(writeErr, "write flame.yaml: %s", manifestPath)
	}
	fmt.Fprintf(opts.Stdout, "flame init: wrote %s (source=%s, version=%s)\n", manifestPath, source, version)
	return nil
}

func resolveValues(opts *RunOptions, defaultVersion string) (source, version string, err error) {
	if opts.Yes {
		return chooseValue(opts.SourceOverride, DefaultSource),
			chooseValue(opts.VersionOverride, defaultVersion),
			nil
	}
	reader := bufio.NewReader(opts.Stdin)
	source, err = resolveField(reader, opts.Stdout, "flame source", opts.SourceOverride, DefaultSource)
	if err != nil {
		return "", "", err
	}
	version, err = resolveField(reader, opts.Stdout, "flame version", opts.VersionOverride, defaultVersion)
	if err != nil {
		return "", "", err
	}
	return source, version, nil
}

// resolveField は (override が空なら) 対話 prompt を 1 回出して 1 行読み取り、 入力が空なら default を返す。 npm init と同じ「default 提示 + Enter で採用 + 任意の値で override」 のセマンティクスを取る。 stdin が EOF の場合 (= 非対話用途で pipe された場合) も「Enter 押下と同じ = default 採用」 として扱う。
func resolveField(reader *bufio.Reader, stdout io.Writer, label, override, defaultValue string) (string, error) {
	if override != "" {
		return override, nil
	}
	fmt.Fprintf(stdout, "%s: (%s) ", label, defaultValue)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", ex.Wrapf(err, "read %s", label)
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return defaultValue, nil
	}
	return trimmed, nil
}

func chooseValue(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

// deriveDefaultVersion は当該 binary 自身の version (`flame --version` の値 = root.Version) を flame.yaml に書き出す形式に整形する。 release build (ldflags 経由) では `1.2.3` 形式で埋め込まれるため `v1.2.3` に前置する。 既に `v` prefix 付きの場合はそのまま採用。 dev build (`0.0.0-dev` 等) は schema (^v\d+\.\d+\.\d+$) に conform しないため placeholder `v0.0.0` を返す (利用者が release tag に手動で書き換える経路)。
func deriveDefaultVersion(binaryVersion string) string {
	trimmed := strings.TrimSpace(binaryVersion)
	if strings.HasPrefix(trimmed, "v") && semverPattern.MatchString(strings.TrimPrefix(trimmed, "v")) {
		return trimmed
	}
	if semverPattern.MatchString(trimmed) {
		return "v" + trimmed
	}
	return PlaceholderVersion
}

// renderManifest は最小 flame.yaml を生成する。 IDE 補完 / 即時 lint のため先頭に schema 参照 directive を付与する ([FLM_FEA_0003](../../../../vendor/flame/docs/adr/feature/FLM_FEA_0003__harness.md) §schema の機械可読化)。 yaml.Marshal を使わず string templating で組み立てるのは、 schema 参照 directive が YAML コメントであり Marshal 経路では 1 行目に置く保証ができないため。
func renderManifest(source, version string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml\n")
	b.WriteString("flame:\n")
	fmt.Fprintf(&b, "  source: %s\n", source)
	fmt.Fprintf(&b, "  version: %s\n", version)
	return b.String()
}
