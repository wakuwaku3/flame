package path_base

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

type fakeGH struct {
	createErr     error
	addErr        error
	listLabelsErr error
	existing      []string
	createCalls   []string
	addCalls      [][2]string
}

func (f *fakeGH) listLabels(_ context.Context, _ string) ([]string, error) {
	return f.existing, f.listLabelsErr
}

func (f *fakeGH) createLabel(_ context.Context, _, label, _, _ string) error {
	f.createCalls = append(f.createCalls, label)
	return f.createErr
}

func (f *fakeGH) addPRLabel(_ context.Context, _, prNumber, label string) error {
	f.addCalls = append(f.addCalls, [2]string{prNumber, label})
	return f.addErr
}

//nolint:paralleltest // env / cwd 変更を伴うため parallel 不可
func TestDoRun(t *testing.T) {
	mkModule := func(t *testing.T, root, name string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Join(root, name), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(root, name, "go.mod"), []byte("module x\n"), 0o600))
	}

	t.Run("module path に該当する変更があれば既存 label でも create 無しで add する", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		mkModule(t, root, "cli")
		mkModule(t, root, "lib")
		t.Chdir(root)
		t.Setenv("FILES_JSON", `["cli/main.go","docs/README.md"]`)
		t.Setenv("PR_NUMBER", "42")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeGH{existing: []string{"module/cli", "bug"}, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, op.createCalls, "既存 label の場合 create は呼ばれない")
		assert.Equal(t, [][2]string{{"42", "module/cli"}}, op.addCalls)
		assert.Contains(t, stdout.String(), "adding label 'module/cli' to PR #42")
	})

	t.Run("FILES_JSON が空配列なら何もしない", func(t *testing.T) {
		// Arrange
		t.Chdir(t.TempDir())
		t.Setenv("FILES_JSON", "[]")
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, op.createCalls)
		assert.Empty(t, op.addCalls)
		assert.Equal(t, "no changed files; nothing to label\n", stdout.String())
	})

	t.Run("module 不在 label は create + add される", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		mkModule(t, root, "lib")
		t.Chdir(root)
		t.Setenv("FILES_JSON", `["lib/foo.go"]`)
		t.Setenv("PR_NUMBER", "7")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []string{"module/lib"}, op.createCalls)
		assert.Equal(t, [][2]string{{"7", "module/lib"}}, op.addCalls)
	})

	// 必須 env 4 種それぞれの未設定を独立 subtest として確認することで、 command.go の検査順序を test に焼き付けつつ regression を 1 ケース 1 違反で局所化する。
	envCases := []struct {
		missing string
	}{
		{missing: "FILES_JSON"},
		{missing: "PR_NUMBER"},
		{missing: "GITHUB_REPOSITORY"},
		{missing: "GH_TOKEN"},
	}
	for _, tc := range envCases {
		t.Run("env "+tc.missing+" 未設定なら exit 1 で stderr にメッセージを出す", func(t *testing.T) {
			// Arrange
			t.Chdir(t.TempDir())
			envs := map[string]string{
				"FILES_JSON":        `["cli/main.go"]`,
				"PR_NUMBER":         "1",
				"GITHUB_REPOSITORY": "owner/repo",
				"GH_TOKEN":          "x",
			}
			envs[tc.missing] = ""
			for k, v := range envs {
				t.Setenv(k, v)
			}
			op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: nil}
			var stdout, stderr bytes.Buffer

			// Act
			err := doRun(t.Context(), &stdout, &stderr, op)

			// Assert
			code, ok := clix.ExitCodeOf(err)
			require.True(t, ok)
			assert.Equal(t, exitCodeFailure, code)
			assert.Equal(t, tc.missing+" must be set\n", stderr.String())
			assert.Empty(t, stdout.String())
		})
	}

	t.Run("FILES_JSON が JSON array でないなら error メッセージを stderr に出して exit 1", func(t *testing.T) {
		// Arrange
		t.Chdir(t.TempDir())
		t.Setenv("FILES_JSON", `not-json`)
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, exitCodeFailure, code)
		assert.Equal(t, "error: FILES_JSON must be a JSON array (got: not-json)\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("cwd に go module が無ければ何もせず stdout に通知して終わる", func(t *testing.T) {
		// Arrange
		t.Chdir(t.TempDir())
		t.Setenv("FILES_JSON", `["cli/main.go"]`)
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, op.createCalls)
		assert.Empty(t, op.addCalls)
		assert.Equal(t, "no go modules found; nothing to label\n", stdout.String())
	})

	t.Run("changed files が module 外のみなら listLabels を呼ばず stdout に通知して終わる", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		mkModule(t, root, "cli")
		t.Chdir(root)
		t.Setenv("FILES_JSON", `["docs/README.md",".github/workflows/x.yaml"]`)
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		// listLabels が呼ばれた場合に検出するため、 fake は listLabels で error を返す設定にする (呼ばれなければ無視される)。
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: ex.Errorf("must not be called")}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, op.createCalls)
		assert.Empty(t, op.addCalls)
		assert.Equal(t, "no module-scoped changes; nothing to label\n", stdout.String())
	})

	t.Run("listLabels 失敗は error を返して伝搬する", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		mkModule(t, root, "cli")
		t.Chdir(root)
		t.Setenv("FILES_JSON", `["cli/main.go"]`)
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		listErr := ex.Errorf("gh label list failed")
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: listErr}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.ErrorIs(t, err, listErr)
		assert.Empty(t, op.createCalls)
		assert.Empty(t, op.addCalls)
	})

	t.Run("createLabel 失敗は error を返し addPRLabel に進まない", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		mkModule(t, root, "cli")
		t.Chdir(root)
		t.Setenv("FILES_JSON", `["cli/main.go"]`)
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		createErr := ex.Errorf("create label failed")
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: createErr, addErr: nil, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.ErrorIs(t, err, createErr)
		assert.Equal(t, []string{"module/cli"}, op.createCalls)
		assert.Empty(t, op.addCalls)
	})

	t.Run("addPRLabel 失敗は error を返す", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		mkModule(t, root, "cli")
		t.Chdir(root)
		t.Setenv("FILES_JSON", `["cli/main.go"]`)
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		addErr := ex.Errorf("add label failed")
		op := &fakeGH{existing: []string{"module/cli"}, createCalls: nil, addCalls: nil, createErr: nil, addErr: addErr, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.ErrorIs(t, err, addErr)
		assert.Empty(t, op.createCalls)
		assert.Equal(t, [][2]string{{"1", "module/cli"}}, op.addCalls)
	})

	t.Run("複数 module が辞書順で create + add される", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		// listGoModules は ReadDir + sort.Strings で辞書順を保証するため、 作成順を逆にしても出力順序が安定することを test で固定する。
		mkModule(t, root, "lib")
		mkModule(t, root, "cli")
		t.Chdir(root)
		t.Setenv("FILES_JSON", `["lib/foo.go","cli/bar.go"]`)
		t.Setenv("PR_NUMBER", "9")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeGH{existing: nil, createCalls: nil, addCalls: nil, createErr: nil, addErr: nil, listLabelsErr: nil}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []string{"module/cli", "module/lib"}, op.createCalls)
		assert.Equal(t, [][2]string{{"9", "module/cli"}, {"9", "module/lib"}}, op.addCalls)
	})
}

//nolint:paralleltest // env / cwd 変更を伴うため parallel 不可
func TestRun(t *testing.T) {
	t.Run("flame ci label path-base は New()/run() 経由で起動して module 不在 path で exit 0 を返す", func(t *testing.T) {
		// Arrange
		// run() の execOps 内蔵経路を実 gh 起動なしで通すため、 module 0 件 (= listLabels が呼ばれない) で exit 0 になる cwd を用意する。
		t.Chdir(t.TempDir())
		t.Setenv("FILES_JSON", `["cli/main.go"]`)
		t.Setenv("PR_NUMBER", "1")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(New())
		fake := clix.NewFakeIO(t, []string{"path-base"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "no go modules found; nothing to label\n", "")
	})
}
