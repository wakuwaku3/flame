package lib

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/release/ghapi"
	"github.com/wakuwaku3/flame/lib/clix"
)

// fakeSystemOps は systemOps interface を満たす test 用 fake (FLM_APP_0009 §mock を採用しない / fake を採用する)。 production が git / gh で行う副作用を bool / 履歴 slice で in-memory 再現し、 test 内から call 履歴と error を直接観測 / 注入する。
type fakeSystemOps struct {
	configureErr   error
	tagPushErr     error
	releaseErr     error
	tagPushCalls   [][]string
	releaseCalls   [][]string
	configureCalls int
	tagLocal       bool
	tagRemote      bool
}

func (f *fakeSystemOps) configureGitIdentity(_ context.Context, _ string) error {
	f.configureCalls++
	return f.configureErr
}

func (f *fakeSystemOps) tagExistsLocal(_ context.Context, _, _ string) bool  { return f.tagLocal }
func (f *fakeSystemOps) tagExistsRemote(_ context.Context, _, _ string) bool { return f.tagRemote }

func (f *fakeSystemOps) gitTagAndPush(_ context.Context, repoRoot, tag, commitSHA string) error {
	f.tagPushCalls = append(f.tagPushCalls, []string{repoRoot, tag, commitSHA})
	return f.tagPushErr
}

func (f *fakeSystemOps) ghReleaseCreate(_ context.Context, tag, title, notesPath, commitSHA string, assets []string) error {
	const fixedHead = 4
	call := make([]string, 0, fixedHead+len(assets))
	call = append(call, tag, title, notesPath, commitSHA)
	call = append(call, assets...)
	f.releaseCalls = append(f.releaseCalls, call)
	return f.releaseErr
}

// TestDoRun は `flame ci release lib` endpoint (= doRun) の主要 7 振る舞いを service-level test で検証する (FLM_APP_0009 §service-level test を主軸とする)。 t.Setenv / t.Chdir を多用するため t.Parallel は採用しない。
//
//nolint:paralleltest // t.Setenv / t.Chdir を多用するため parallel 不可。
func TestDoRun(t *testing.T) {
	const (
		moduleName       = "lib"
		commitSHA        = "HEADSHA"
		repoFull         = "owner/name"
		releasesPath     = "/repos/owner/name/releases?per_page=100"
		comparePathV1Lib = "/repos/owner/name/compare/lib/v1.0.0...HEADSHA"
	)

	// setupRepo は repoRoot に配布対象 module (go.mod + 公開 surface 1 件) を組み立てる。 公開 surface が 0 件の package は speclib.EmitTo が pkgs に含めない設計なので、 spec 抽出経路を通すために `func Hello()` を 1 件持たせる。
	setupRepo := func(tb testing.TB) string {
		tb.Helper()
		repoRoot := tb.TempDir()
		moduleDir := filepath.Join(repoRoot, moduleName)
		require.NoError(tb, os.MkdirAll(moduleDir, fsperm.Dir))
		require.NoError(tb, os.WriteFile(filepath.Join(moduleDir, "go.mod"),
			[]byte("module example.com/lib\n\ngo 1.26\n"), fsperm.File))
		require.NoError(tb, os.WriteFile(filepath.Join(moduleDir, "lib.go"),
			[]byte("package lib\n\nfunc Hello() string { return \"hi\" }\n"), fsperm.File))
		return repoRoot
	}

	t.Run("GITHUB_REPOSITORY 未設定なら error と stderr メッセージ + exit code 1", func(t *testing.T) {
		// Arrange
		t.Setenv("GITHUB_REPOSITORY", "")
		t.Setenv("ACTOR", "")
		t.Setenv("DRY_RUN", "")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		t.Chdir(setupRepo(t))
		gh := ghapi.NewStub(t)
		fsys := &fakeSystemOps{configureErr: nil, tagPushErr: nil, releaseErr: nil, tagPushCalls: nil, releaseCalls: nil, configureCalls: 0, tagLocal: false, tagRemote: false}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, moduleName, commitSHA, gh, fsys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Empty(t, stdout.String())
		assert.Equal(t, "GITHUB_REPOSITORY must be set\n", stderr.String())
		assert.Empty(t, gh.APICalls)
		assert.Equal(t, 0, fsys.configureCalls)
		assert.Empty(t, fsys.tagPushCalls)
		assert.Empty(t, fsys.releaseCalls)
	})

	t.Run("ACTOR 未設定 (dry_run=false) なら error と stderr メッセージ + exit code 1", func(t *testing.T) {
		// Arrange
		t.Setenv("GITHUB_REPOSITORY", repoFull)
		t.Setenv("ACTOR", "")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		t.Chdir(setupRepo(t))
		gh := ghapi.NewStub(t)
		fsys := &fakeSystemOps{configureErr: nil, tagPushErr: nil, releaseErr: nil, tagPushCalls: nil, releaseCalls: nil, configureCalls: 0, tagLocal: false, tagRemote: false}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, moduleName, commitSHA, gh, fsys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Empty(t, stdout.String())
		assert.Equal(t, "ACTOR must be set\n", stderr.String())
		assert.Empty(t, gh.APICalls)
		assert.Equal(t, 0, fsys.configureCalls)
	})

	t.Run("dry run mode は tag push と gh release create を skip し stdout に DRY RUN プレビューを出す", func(t *testing.T) {
		// Arrange
		t.Setenv("GITHUB_REPOSITORY", repoFull)
		t.Setenv("ACTOR", "")
		t.Setenv("DRY_RUN", "true")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		t.Chdir(setupRepo(t))
		gh := ghapi.NewStub(t)
		// release 一覧 0 件 → prior 不在 → 初版扱い (Bump=initial / Next=1.0.0) で ReleaseDownload / compare API のいずれも呼ばれない経路に落とす。
		gh.APIResponses[releasesPath] = []byte(`[]`)
		fsys := &fakeSystemOps{configureErr: nil, tagPushErr: nil, releaseErr: nil, tagPushCalls: nil, releaseCalls: nil, configureCalls: 0, tagLocal: false, tagRemote: false}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, moduleName, commitSHA, gh, fsys)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, stderr.String())
		assert.Contains(t, stdout.String(), "release plan: module=lib")
		assert.Contains(t, stdout.String(), "bump=initial next=1.0.0 dry_run=true")
		assert.Contains(t, stdout.String(), "DRY RUN: skipping tag push and gh release create for lib/v1.0.0")
		assert.Contains(t, stdout.String(), "=== release notes preview (DRY RUN) ===")
		assert.Equal(t, 0, fsys.configureCalls, "dry_run は configureGitIdentity を呼ばない")
		assert.Empty(t, fsys.tagPushCalls)
		assert.Empty(t, fsys.releaseCalls)
	})

	t.Run("tagExistsLocal=true なら error と stderr メッセージ + exit code 1", func(t *testing.T) {
		// Arrange
		t.Setenv("GITHUB_REPOSITORY", repoFull)
		t.Setenv("ACTOR", "actor1")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		t.Chdir(setupRepo(t))
		gh := ghapi.NewStub(t)
		gh.APIResponses[releasesPath] = []byte(`[]`)
		fsys := &fakeSystemOps{configureErr: nil, tagPushErr: nil, releaseErr: nil, tagPushCalls: nil, releaseCalls: nil, configureCalls: 0, tagLocal: true, tagRemote: false}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, moduleName, commitSHA, gh, fsys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Equal(t, "error: tag 'lib/v1.0.0' already exists locally\n", stderr.String())
		assert.Equal(t, 1, fsys.configureCalls, "dry_run=false なので configureGitIdentity が 1 回呼ばれる")
		assert.Empty(t, fsys.tagPushCalls, "local tag conflict なら gitTagAndPush は呼ばれない")
		assert.Empty(t, fsys.releaseCalls)
	})

	t.Run("tagExistsRemote=true なら error と stderr メッセージ + exit code 1", func(t *testing.T) {
		// Arrange
		t.Setenv("GITHUB_REPOSITORY", repoFull)
		t.Setenv("ACTOR", "actor1")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		t.Chdir(setupRepo(t))
		gh := ghapi.NewStub(t)
		gh.APIResponses[releasesPath] = []byte(`[]`)
		fsys := &fakeSystemOps{configureErr: nil, tagPushErr: nil, releaseErr: nil, tagPushCalls: nil, releaseCalls: nil, configureCalls: 0, tagLocal: false, tagRemote: true}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, moduleName, commitSHA, gh, fsys)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Equal(t, "error: tag 'lib/v1.0.0' already exists on origin\n", stderr.String())
		assert.Empty(t, fsys.tagPushCalls)
		assert.Empty(t, fsys.releaseCalls)
	})

	t.Run("priorTag 有り + module 配下に file change 無しなら skip release で exit 0", func(t *testing.T) {
		// Arrange
		t.Setenv("GITHUB_REPOSITORY", repoFull)
		t.Setenv("ACTOR", "actor1")
		t.Setenv("DRY_RUN", "false")
		// PRIOR_TAG_OVERRIDE で release list API を skip し、 prior=1.0.0 の経路に強制する。 ReleaseDownload は DownloadAssets 未登録のため fail し warn を経由して bump=patch に fallback する (production 想定の同経路を再現)。
		t.Setenv("PRIOR_TAG_OVERRIDE", "lib/v1.0.0")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		t.Chdir(setupRepo(t))
		gh := ghapi.NewStub(t)
		// compare API は files が当該 module dir prefix にマッチしない → HasModuleChangesSincePriorTag が false を返し skip 経路。
		gh.APIResponses[comparePathV1Lib] = []byte(`{"total_commits":1,"commits":[{"sha":"a"}],"files":[{"filename":"docs/x.md"}]}`)
		fsys := &fakeSystemOps{configureErr: nil, tagPushErr: nil, releaseErr: nil, tagPushCalls: nil, releaseCalls: nil, configureCalls: 0, tagLocal: false, tagRemote: false}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, moduleName, commitSHA, gh, fsys)

		// Assert
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "skip release: no file changes under lib/ since lib/v1.0.0")
		assert.Equal(t, 0, fsys.configureCalls, "skip 経路では configureGitIdentity は呼ばれない")
		assert.Empty(t, fsys.tagPushCalls)
		assert.Empty(t, fsys.releaseCalls)
	})

	t.Run("happy path: 初版 release で gitTagAndPush と ghReleaseCreate が 1 回ずつ呼ばれる", func(t *testing.T) {
		// Arrange
		t.Setenv("GITHUB_REPOSITORY", repoFull)
		t.Setenv("ACTOR", "actor1")
		t.Setenv("DRY_RUN", "false")
		t.Setenv("PRIOR_TAG_OVERRIDE", "")
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		repoRoot := setupRepo(t)
		t.Chdir(repoRoot)
		gh := ghapi.NewStub(t)
		gh.APIResponses[releasesPath] = []byte(`[]`)
		fsys := &fakeSystemOps{configureErr: nil, tagPushErr: nil, releaseErr: nil, tagPushCalls: nil, releaseCalls: nil, configureCalls: 0, tagLocal: false, tagRemote: false}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, moduleName, commitSHA, gh, fsys)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, stderr.String())
		assert.Contains(t, stdout.String(), "bump=initial next=1.0.0 dry_run=false")
		assert.Equal(t, 1, fsys.configureCalls)
		require.Len(t, fsys.tagPushCalls, 1)
		assert.Equal(t, []string{repoRoot, "lib/v1.0.0", commitSHA}, fsys.tagPushCalls[0])
		require.Len(t, fsys.releaseCalls, 1)
		assert.Equal(t, "lib/v1.0.0", fsys.releaseCalls[0][0])
		assert.Equal(t, "lib v1.0.0", fsys.releaseCalls[0][1])
		assert.Equal(t, commitSHA, fsys.releaseCalls[0][3])
		assert.Len(t, fsys.releaseCalls[0], 5, "release call は spec asset 1 件を含めて全 5 要素")
	})
}
