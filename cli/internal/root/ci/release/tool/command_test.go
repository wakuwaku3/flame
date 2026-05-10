package tool

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/release/ghapi"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

// fakeSystemOps は systemOps を test 内で差し替える fake (FLM_APP_0009 §mock を採用しない / fake を採用する)。
// buildBinary / emitSpec は archive 化 / checksums 計算が実 file system 上で完結するように、
// outPath へ dummy bytes を実際に書き出す。 git / gh 系は呼び出し履歴の記録のみ行う。
type fakeSystemOps struct {
	ghReleaseAssets    []string
	buildOutPaths      []string
	emitOutPaths       []string
	configureCalls     int
	gitTagAndPushCalls int
	tagLocal           bool
	tagRemote          bool
}

func (f *fakeSystemOps) configureGitIdentity(_ context.Context, _ string) error {
	f.configureCalls++
	return nil
}

func (f *fakeSystemOps) tagExistsLocal(_ context.Context, _, _ string) bool {
	return f.tagLocal
}

func (f *fakeSystemOps) tagExistsRemote(_ context.Context, _, _ string) bool {
	return f.tagRemote
}

func (f *fakeSystemOps) gitTagAndPush(_ context.Context, _, _, _ string) error {
	f.gitTagAndPushCalls++
	return nil
}

func (f *fakeSystemOps) ghReleaseCreate(_ context.Context, _, _, _, _ string, assets []string) error {
	f.ghReleaseAssets = append([]string{}, assets...)
	return nil
}

func (f *fakeSystemOps) buildBinary(_ context.Context, _, _, outPath string, _ []string) error {
	f.buildOutPaths = append(f.buildOutPaths, outPath)
	// 後続 makeArchive / writeChecksums が実 file を要求するため dummy bytes を必ず書き出す。
	return ex.Wrap(os.WriteFile(outPath, []byte("dummy-binary"), fsperm.File))
}

func (f *fakeSystemOps) emitSpec(_ context.Context, _, outPath string) error {
	f.emitOutPaths = append(f.emitOutPaths, outPath)
	return ex.Wrap(os.WriteFile(outPath, []byte(`{"path":"app","subcommands":[],"flags":[]}`), fsperm.File))
}

// chdirTemp は cwd を t.TempDir() に切り替える。 doRun 内 os.Getwd() を経由するため、 各 test ケースで repoRoot を独立させる。
func chdirTemp(tb testing.TB) {
	tb.Helper()
	tb.Chdir(tb.TempDir())
}

//nolint:paralleltest // env / cwd を変更するため parallel 不可
func TestDoRun(t *testing.T) {
	t.Run("GITHUB_REPOSITORY 未設定 → stderr + exit 1", func(t *testing.T) {
		// Arrange
		chdirTemp(t)
		t.Setenv("GITHUB_REPOSITORY", "")
		t.Setenv("DRY_RUN", "true")
		t.Setenv("ACTOR", "")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		gh := ghapi.NewStub(t)
		sys := &fakeSystemOps{
			ghReleaseAssets:    nil,
			buildOutPaths:      nil,
			emitOutPaths:       nil,
			configureCalls:     0,
			gitTagAndPushCalls: 0,
			tagLocal:           false,
			tagRemote:          false,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(context.Background(), &stdout, &stderr, "module", "appdir", "app", "HEADSHA", gh, sys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Equal(t, "GITHUB_REPOSITORY must be set\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("ACTOR 未設定 (dry_run=false) → stderr + exit 1", func(t *testing.T) {
		// Arrange
		chdirTemp(t)
		t.Setenv("GITHUB_REPOSITORY", "owner/name")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("ACTOR", "")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		gh := ghapi.NewStub(t)
		sys := &fakeSystemOps{
			ghReleaseAssets:    nil,
			buildOutPaths:      nil,
			emitOutPaths:       nil,
			configureCalls:     0,
			gitTagAndPushCalls: 0,
			tagLocal:           false,
			tagRemote:          false,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(context.Background(), &stdout, &stderr, "module", "appdir", "app", "HEADSHA", gh, sys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Equal(t, "ACTOR must be set\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("dry_run 経路: tag push と gh release create を skip し DRY RUN preview を stdout に出す", func(t *testing.T) {
		// Arrange
		chdirTemp(t)
		t.Setenv("GITHUB_REPOSITORY", "owner/name")
		t.Setenv("DRY_RUN", "true")
		t.Setenv("ACTOR", "")
		t.Setenv("PRIOR_TAG_OVERRIDE", "app/v1.2.3")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		gh := ghapi.NewStub(t)
		// HasModuleChangesSincePriorTag 用: module/ 配下の変更ありで release を進ませる。
		gh.APIResponses["/repos/owner/name/compare/app/v1.2.3...HEADSHA"] = []byte(
			`{"total_commits":1,"commits":[{"sha":"abc"}],"files":[{"filename":"module/foo.go"}]}`,
		)
		// prior spec download は fixture 不在で error → patch bump fallback。
		sys := &fakeSystemOps{
			ghReleaseAssets:    nil,
			buildOutPaths:      nil,
			emitOutPaths:       nil,
			configureCalls:     0,
			gitTagAndPushCalls: 0,
			tagLocal:           false,
			tagRemote:          false,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(context.Background(), &stdout, &stderr, "module", "appdir", "app", "HEADSHA", gh, sys)

		// Assert
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "release plan: app=app prior=1.2.3 bump=patch next=1.2.4 dry_run=true")
		assert.Contains(t, stdout.String(), "DRY RUN: skipping tag push and gh release create for app/v1.2.4")
		assert.Contains(t, stdout.String(), "=== release notes preview (DRY RUN) ===")
		assert.Equal(t, 0, sys.configureCalls, "dry_run では git identity 設定を呼ばない")
		assert.Equal(t, 0, sys.gitTagAndPushCalls, "dry_run では tag push を呼ばない")
		assert.Empty(t, sys.ghReleaseAssets, "dry_run では gh release create を呼ばない")
		assert.Len(t, sys.buildOutPaths, 1, "dry_run では cross-build を行わず spec 用 build 1 回のみ")
	})

	t.Run("tagExistsLocal=true → stderr + exit 1", func(t *testing.T) {
		// Arrange
		chdirTemp(t)
		t.Setenv("GITHUB_REPOSITORY", "owner/name")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("ACTOR", "octocat")
		t.Setenv("PRIOR_TAG_OVERRIDE", "app/v1.2.3")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		gh := ghapi.NewStub(t)
		gh.APIResponses["/repos/owner/name/compare/app/v1.2.3...HEADSHA"] = []byte(
			`{"total_commits":1,"commits":[{"sha":"abc"}],"files":[{"filename":"module/foo.go"}]}`,
		)
		sys := &fakeSystemOps{
			ghReleaseAssets:    nil,
			buildOutPaths:      nil,
			emitOutPaths:       nil,
			configureCalls:     0,
			gitTagAndPushCalls: 0,
			tagLocal:           true,
			tagRemote:          false,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(context.Background(), &stdout, &stderr, "module", "appdir", "app", "HEADSHA", gh, sys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Contains(t, stderr.String(), "error: tag 'app/v1.2.4' already exists locally")
		assert.Equal(t, 0, sys.gitTagAndPushCalls)
		assert.Empty(t, sys.ghReleaseAssets)
	})

	t.Run("tagExistsRemote=true → stderr + exit 1", func(t *testing.T) {
		// Arrange
		chdirTemp(t)
		t.Setenv("GITHUB_REPOSITORY", "owner/name")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("ACTOR", "octocat")
		t.Setenv("PRIOR_TAG_OVERRIDE", "app/v1.2.3")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		gh := ghapi.NewStub(t)
		gh.APIResponses["/repos/owner/name/compare/app/v1.2.3...HEADSHA"] = []byte(
			`{"total_commits":1,"commits":[{"sha":"abc"}],"files":[{"filename":"module/foo.go"}]}`,
		)
		sys := &fakeSystemOps{
			ghReleaseAssets:    nil,
			buildOutPaths:      nil,
			emitOutPaths:       nil,
			configureCalls:     0,
			gitTagAndPushCalls: 0,
			tagLocal:           false,
			tagRemote:          true,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(context.Background(), &stdout, &stderr, "module", "appdir", "app", "HEADSHA", gh, sys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Contains(t, stderr.String(), "error: tag 'app/v1.2.4' already exists on origin")
		assert.Equal(t, 0, sys.gitTagAndPushCalls)
		assert.Empty(t, sys.ghReleaseAssets)
	})

	t.Run("priorTag 有り + module 配下 file 変更無し → release skip", func(t *testing.T) {
		// Arrange
		chdirTemp(t)
		t.Setenv("GITHUB_REPOSITORY", "owner/name")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("ACTOR", "octocat")
		t.Setenv("PRIOR_TAG_OVERRIDE", "app/v1.2.3")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		gh := ghapi.NewStub(t)
		// module/ 配下に変更が無い compare (docs/ のみ) → skip 経路。
		gh.APIResponses["/repos/owner/name/compare/app/v1.2.3...HEADSHA"] = []byte(
			`{"total_commits":1,"commits":[{"sha":"abc"}],"files":[{"filename":"docs/x.md"}]}`,
		)
		sys := &fakeSystemOps{
			ghReleaseAssets:    nil,
			buildOutPaths:      nil,
			emitOutPaths:       nil,
			configureCalls:     0,
			gitTagAndPushCalls: 0,
			tagLocal:           false,
			tagRemote:          false,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(context.Background(), &stdout, &stderr, "module", "appdir", "app", "HEADSHA", gh, sys)

		// Assert
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "skip release: no file changes under module/ since app/v1.2.3")
		assert.Equal(t, 0, sys.configureCalls, "skip 時は git identity 設定を呼ばない")
		assert.Equal(t, 0, sys.gitTagAndPushCalls)
		assert.Empty(t, sys.ghReleaseAssets)
	})

	t.Run("happy path: build / spec / cross-archives / gitTagAndPush / ghReleaseCreate を順に呼ぶ", func(t *testing.T) {
		// Arrange
		chdirTemp(t)
		t.Setenv("GITHUB_REPOSITORY", "owner/name")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("ACTOR", "octocat")
		t.Setenv("PRIOR_TAG_OVERRIDE", "app/v1.2.3")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		gh := ghapi.NewStub(t)
		gh.APIResponses["/repos/owner/name/compare/app/v1.2.3...HEADSHA"] = []byte(
			`{"total_commits":1,"commits":[{"sha":"abc"}],"files":[{"filename":"module/foo.go"}]}`,
		)
		sys := &fakeSystemOps{
			ghReleaseAssets:    nil,
			buildOutPaths:      nil,
			emitOutPaths:       nil,
			configureCalls:     0,
			gitTagAndPushCalls: 0,
			tagLocal:           false,
			tagRemote:          false,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(context.Background(), &stdout, &stderr, "module", "appdir", "app", "HEADSHA", gh, sys)

		// Assert
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "release plan: app=app prior=1.2.3 bump=patch next=1.2.4 dry_run=false")
		assert.Equal(t, 1, sys.configureCalls)
		assert.Equal(t, 1, sys.gitTagAndPushCalls)
		// buildBinary 呼び出し: spec 抽出用 1 回 + cross-build 6 platform = 7 回。
		assert.Len(t, sys.buildOutPaths, 1+len(targetPlatforms))
		assert.Len(t, sys.emitOutPaths, 1)
		// ghReleaseCreate に渡る asset: 6 archive + SHA256SUMS + cli-spec.json。
		assert.Len(t, sys.ghReleaseAssets, len(targetPlatforms)+2)
		names := make([]string, 0, len(sys.ghReleaseAssets))
		for _, p := range sys.ghReleaseAssets {
			names = append(names, filepath.Base(p))
		}
		sort.Strings(names)
		assert.Contains(t, names, "SHA256SUMS")
		assert.Contains(t, names, specAssetName)
		hasArchive := false
		for _, n := range names {
			if strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".zip") {
				hasArchive = true
				break
			}
		}
		assert.True(t, hasArchive, "tar.gz / zip archive が少なくとも 1 件含まれる")
	})
}

// flattenToolSpec は CLI spec JSON を「<path> sub <name>」 / 「<path> flag <name>」 の flat map に変換する pure helper のため、
// service-level test では再現しにくい入出力 (subcommand / flag 網羅、 不正 JSON) のみ unit test として薄く確認する (FLM_APP_0009 §unit test の責務)。
//
//nolint:paralleltest // 同 file 内 test と整合させるため direct subtest 形式
func TestFlattenToolSpec(t *testing.T) {
	t.Run("subcommand と flag を flat map に展開する", func(t *testing.T) {
		// Arrange
		const specJSON = `{
			"path": "root",
			"subcommands": [
				{"name": "sub", "path": "root sub", "subcommands": [], "flags": [{"name": "verbose", "type": "bool", "required": false}]}
			],
			"flags": [{"name": "config", "type": "string", "required": true}]
		}`

		// Act
		got, err := flattenToolSpec([]byte(specJSON))

		// Assert
		require.NoError(t, err)
		expected := map[string]string{
			"root sub sub":          "",
			"root flag config":      "string:true",
			"root sub flag verbose": "bool:false",
		}
		assert.Equal(t, expected, got)
	})

	t.Run("不正 JSON は error を返す", func(t *testing.T) {
		// Arrange
		const specJSON = `{not-json`

		// Act
		got, err := flattenToolSpec([]byte(specJSON))

		// Assert
		require.Error(t, err)
		assert.Nil(t, got)
	})
}
