package lint

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/lib/clix"
)

// fakeOps は test 内で golangci-lint / go mod tidy 系の副作用を差し替える fake (FLM_APP_0009 §mock を採用しない / fake を採用する)。
type fakeOps struct {
	lintErr        error
	tidyApplyOut   []byte
	tidyApplyErr   error
	tidyDiffOutput string
	tidyDiffErr    error
	lintCalls      []lintCall
	tidyApplyDirs  []string
	tidyDiffDirs   []string
}

type lintCall struct {
	moduleRoot string
	args       []string
}

func (f *fakeOps) runLint(_ context.Context, moduleRoot string, args []string, _, _ io.Writer) error {
	f.lintCalls = append(f.lintCalls, lintCall{moduleRoot: moduleRoot, args: args})
	return f.lintErr
}

func (f *fakeOps) runTidyApply(_ context.Context, moduleRoot string) ([]byte, error) {
	f.tidyApplyDirs = append(f.tidyApplyDirs, moduleRoot)
	return f.tidyApplyOut, f.tidyApplyErr
}

func (f *fakeOps) runTidyDiff(_ context.Context, moduleRoot string, _ io.Writer) (string, error) {
	f.tidyDiffDirs = append(f.tidyDiffDirs, moduleRoot)
	return f.tidyDiffOutput, f.tidyDiffErr
}

// setupModule は repoRoot 配下に mod/go.mod と mod/pkgRel ディレクトリを作り、 cwd を repoRoot に固定する (FLM_APP_0009 §test helper signature)。 endpoint が `os.Getwd()` で repoRoot を取り、 args の相対 path から module root を辿る経路全体を test ケース内で組み立てる。
func setupModule(tb testing.TB, pkgRel string) (repoRoot, pkgArg string) {
	tb.Helper()
	repoRoot = tb.TempDir()
	moduleRoot := filepath.Join(repoRoot, "mod")
	require.NoError(tb, os.MkdirAll(filepath.Join(moduleRoot, pkgRel), 0o700))
	require.NoError(tb, os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module fake\n\ngo 1.24\n"), 0o600))
	tb.Chdir(repoRoot)
	return repoRoot, "./mod/" + filepath.ToSlash(pkgRel)
}

//nolint:paralleltest // env / cwd 変更を伴うため parallel 不可
func TestDoRun(t *testing.T) {
	t.Run("lint 違反検出 → FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")

		// Arrange
		_, pkgArg := setupModule(t, "pkg")
		op := &fakeOps{
			lintErr:        errors.New("violation"),
			tidyApplyOut:   nil,
			tidyApplyErr:   nil,
			tidyDiffOutput: "",
			tidyDiffErr:    nil,
			lintCalls:      nil,
			tidyApplyDirs:  nil,
			tidyDiffDirs:   nil,
		}
		var stdout, stderr bytes.Buffer
		expectedStderr := "FAIL: " + pkgArg + ": golangci-lint ./pkg が違反を検出した\n"

		// Act
		err := doRun(t.Context(), []string{pkgArg}, &stdout, &stderr, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout.String())
		assert.Equal(t, expectedStderr, stderr.String())
		assert.Len(t, op.lintCalls, 1)
		assert.Equal(t, []string{"run", "./pkg"}, op.lintCalls[0].args)
	})

	t.Run("lint 成功 + tidy clean → exit 0 / 出力なし", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")

		// Arrange
		_, pkgArg := setupModule(t, "pkg")
		op := &fakeOps{
			lintErr:        nil,
			tidyApplyOut:   nil,
			tidyApplyErr:   nil,
			tidyDiffOutput: "",
			tidyDiffErr:    nil,
			lintCalls:      nil,
			tidyApplyDirs:  nil,
			tidyDiffDirs:   nil,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), []string{pkgArg}, &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
		// diagnose mode では tidy apply は呼ばれず diff のみ呼ばれる。
		assert.Empty(t, op.tidyApplyDirs)
		assert.Len(t, op.tidyDiffDirs, 1)
	})

	t.Run("fix mode で go mod tidy 失敗 → FAIL 行 + tidy 出力 + exit code 1", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "fix")

		// Arrange
		_, pkgArg := setupModule(t, "pkg")
		op := &fakeOps{
			lintErr:        nil,
			tidyApplyOut:   []byte("missing go.sum entry"),
			tidyApplyErr:   errors.New("exit status 1"),
			tidyDiffOutput: "",
			tidyDiffErr:    nil,
			lintCalls:      nil,
			tidyApplyDirs:  nil,
			tidyDiffDirs:   nil,
		}
		var stdout, stderr bytes.Buffer
		// tidy 出力末尾が改行を含まないため endpoint が改行を補う経路を踏む (FLM_APP_0009 §出力 / 戻り値全体に対して assertion を行う)。
		expectedStderr := "FAIL: mod: go mod tidy が失敗した:\nmissing go.sum entry\n"

		// Act
		err := doRun(t.Context(), []string{pkgArg}, &stdout, &stderr, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout.String())
		assert.Equal(t, expectedStderr, stderr.String())
		assert.Equal(t, []string{"run", "--fix", "./pkg"}, op.lintCalls[0].args)
	})

	t.Run("tidy diff drift 検出 → FAIL 行 + diff + exit code 1", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")

		// Arrange
		_, pkgArg := setupModule(t, "pkg")
		op := &fakeOps{
			lintErr:        nil,
			tidyApplyOut:   nil,
			tidyApplyErr:   nil,
			tidyDiffOutput: "--- a/go.mod\n+++ b/go.mod\n@@ require\n",
			tidyDiffErr:    nil,
			lintCalls:      nil,
			tidyApplyDirs:  nil,
			tidyDiffDirs:   nil,
		}
		var stdout, stderr bytes.Buffer
		expectedStderr := "FAIL: mod: go.mod / go.sum が tidy 状態でない (go mod tidy で整える):\n--- a/go.mod\n+++ b/go.mod\n@@ require\n"

		// Act
		err := doRun(t.Context(), []string{pkgArg}, &stdout, &stderr, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout.String())
		assert.Equal(t, expectedStderr, stderr.String())
	})

	t.Run("同一 module の複数 package 引数で tidy は 1 回しか呼ばれない", func(t *testing.T) {
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")

		// Arrange
		repoRoot, pkgArgA := setupModule(t, "a")
		require.NoError(t, os.Mkdir(filepath.Join(repoRoot, "mod", "b"), 0o700))
		pkgArgB := "./mod/b"
		op := &fakeOps{
			lintErr:        nil,
			tidyApplyOut:   nil,
			tidyApplyErr:   nil,
			tidyDiffOutput: "",
			tidyDiffErr:    nil,
			lintCalls:      nil,
			tidyApplyDirs:  nil,
			tidyDiffDirs:   nil,
		}
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), []string{pkgArgA, pkgArgB}, &stdout, &stderr, op)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
		assert.Len(t, op.lintCalls, 2)
		// seenModules による dedup が module root 単位で 1 回だけ tidy diff を呼ぶ性質を焼き付ける (FLM_APP_0009 §endpoint の振る舞い決定 layer)。
		assert.Len(t, op.tidyDiffDirs, 1)
	})
}
