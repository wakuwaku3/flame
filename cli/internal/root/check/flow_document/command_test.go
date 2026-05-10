package flow_document_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/flow_document"
	"github.com/wakuwaku3/flame/lib/clix"
)

// TestRun は親 test で 1 回だけ tempdir に cwd を切り替えるため、 親自体は t.Parallel() しない。 t.Chdir は parallel と相互排他で、 同 test 内で両者を呼ぶと Go 公式が panic を投げる仕様 (Go 1.24+)。 paralleltest linter は親 t.Parallel() の不在を false positive として検出するが、 cwd 注入手段が「subcommand 経由 = service-level test」 (FLM_APP_0009) では t.Chdir 以外に無いため、 グローバル無効化ではなく当該 1 行に絞った局所抑制で対処する (FLM_GEN_0006 §局所抑制が真に避けられない場合)。
//
//nolint:paralleltest,tparallel // t.Chdir と t.Parallel() の同時利用は Go 公式仕様で禁止 (上記 doc 参照)。 親非 parallel + 子 parallel の構造に対して両 linter が false positive を出すため局所抑制する
func TestRun(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	t.Run("全 valid な flow ディレクトリなら no-op success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__spec__valid_happy"
		mustWriteIndex(t, root, flowDir)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", filepath.Join(flowDir, "index.md")})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("docs/notes/ 外のパスは FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		target := filepath.Join("docs", "adr", "outside.md")
		require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "adr"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, target), []byte("# foo"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", target})
		expectedStderr := "FAIL: " + target + ": not under docs/notes/ (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("docs/notes/ 直下のファイル指定は FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		target := filepath.Join("docs", "notes", "orphan_file.md")
		require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "notes"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, target), []byte("# orphan"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", target})
		expectedStderr := "FAIL: " + target + ": files directly under docs/notes/ are not allowed; create docs/notes/<dir>/ first (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("'__' 区切りが足りないディレクトリ名は FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__spec_only"
		mustWriteIndex(t, root, flowDir)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", filepath.Join(flowDir, "index.md")})
		expectedStderr := "FAIL: " + flowDir + ": directory name '202605061230__spec_only' must use '__' separators between date / type / title (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("date 形式違反 (12 桁でない) は FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/2026050612__spec__short_date"
		mustWriteIndex(t, root, flowDir)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", filepath.Join(flowDir, "index.md")})
		expectedStderr := "FAIL: " + flowDir + ": date '2026050612' must be 'yyyymmddhhmm' (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("date のレンジ違反 (月 13 / 時 24) は対応する FAIL 行をまとめて出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202613062430__spec__bad_range"
		mustWriteIndex(t, root, flowDir)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", filepath.Join(flowDir, "index.md")})
		expectedStderr := "FAIL: " + flowDir + ": month '13' out of range (FLM_APP_0006)\n" +
			"FAIL: " + flowDir + ": hour '24' out of range (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("type が列挙外なら FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__memo__bad_type"
		mustWriteIndex(t, root, flowDir)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", filepath.Join(flowDir, "index.md")})
		expectedStderr := "FAIL: " + flowDir + ": type 'memo' must be one of: spec, tips, report (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("title が lower snake_case でない (大文字 / ハイフン) なら FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__spec__Bad-Title"
		mustWriteIndex(t, root, flowDir)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", filepath.Join(flowDir, "index.md")})
		expectedStderr := "FAIL: " + flowDir + ": title 'Bad-Title' must be lower snake_case starting with a letter (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("index.md が無い flow ディレクトリは FAIL 行を出す", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__spec__missing_index"
		require.NoError(t, os.MkdirAll(filepath.Join(root, flowDir), 0o700))
		other := filepath.Join(flowDir, "memo.md")
		require.NoError(t, os.WriteFile(filepath.Join(root, other), []byte("memo"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", other})
		expectedStderr := "FAIL: " + flowDir + ": missing index.md entry file (FLM_APP_0006)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("同一 flow ディレクトリ配下の複数ファイル指定は flow ディレクトリ単位で重複排除する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__spec__dedup_target"
		mustWriteIndex(t, root, flowDir)
		require.NoError(t, os.WriteFile(filepath.Join(root, flowDir, "memo.md"), []byte("memo"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{
			"flow-document",
			filepath.Join(flowDir, "index.md"),
			filepath.Join(flowDir, "memo.md"),
		})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("absolute path で渡しても repo-root-relative に正規化して検査する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__spec__abs_path"
		mustWriteIndex(t, root, flowDir)
		// t.Chdir 後の cwd は os.Getwd() の resolve 結果と一致するため、 そこから組み立てた absolute path を repo-root-relative に正規化できる経路を検証する。
		cwd, getwdErr := os.Getwd()
		require.NoError(t, getwdErr)
		abs := filepath.Join(cwd, flowDir, "index.md")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", abs})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("'./' prefix 付きの repo-root-relative パスも正規化して検査する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		flowDir := "docs/notes/202605061230__spec__dot_slash"
		mustWriteIndex(t, root, flowDir)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document", "./" + filepath.Join(flowDir, "index.md")})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("引数なし起動は usage を stderr に出して exit code 2 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(flow_document.New())
		fake := clix.NewFakeIO(t, []string{"flow-document"})
		const expectedStderr = "usage: flame check flow-document <file_under_docs_notes>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})
}

// mustWriteIndex は flow_dir 配下に index.md を作成する組み立て helper (FLM_APP_0009 §test helper signature)。
func mustWriteIndex(tb testing.TB, root, flowDir string) {
	tb.Helper()
	require.NoError(tb, os.MkdirAll(filepath.Join(root, flowDir), 0o700))
	require.NoError(tb, os.WriteFile(filepath.Join(root, flowDir, "index.md"), []byte("# entry"), 0o600))
}
