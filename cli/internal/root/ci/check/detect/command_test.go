package detect

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/lib/clix"
)

// fakeOps は test 内で gh / git / scripts/detect.sh の副作用を差し替える fake (FLM_APP_0009 §mock を採用しない / fake を採用する)。
type fakeOps struct {
	compareJSON  []byte
	diffOutput   []byte
	bucketResult []bucketEntry
	fetchCalls   [][]string
	bucketCalls  [][]string
}

func (f *fakeOps) ghCompare(_ context.Context, _, _, _ string) ([]byte, error) {
	return f.compareJSON, nil
}

func (f *fakeOps) gitFetch(_ context.Context, args ...string) error {
	f.fetchCalls = append(f.fetchCalls, args)
	return nil
}

func (f *fakeOps) gitDiffNamesZ(_ context.Context, _ string) ([]byte, error) {
	return f.diffOutput, nil
}

func (f *fakeOps) bucketize(_ context.Context, files []string) ([]bucketEntry, error) {
	f.bucketCalls = append(f.bucketCalls, files)
	return f.bucketResult, nil
}

//nolint:paralleltest // env 変更を伴うため parallel 不可
func TestDoRun(t *testing.T) {
	t.Run("差分有 → files / matrix / has_work=true を emit", func(t *testing.T) {
		// Arrange
		t.Setenv("BASE_SHA", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111")
		t.Setenv("HEAD_SHA", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeOps{
			compareJSON:  []byte(`{"merge_base_commit":{"sha":"cccc3333cccc3333cccc3333cccc3333cccc3333"},"ahead_by":2}`),
			diffOutput:   []byte("a.go\x00b.md\x00"),
			bucketResult: []bucketEntry{{Checker: "go-lint", Files: []string{"a.go"}}, {Checker: "document", Files: []string{"b.md"}}},
			fetchCalls:   nil,
			bucketCalls:  nil,
		}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer
		const expectedFile = `files=["a.go","b.md"]
matrix={"include":[{"checker":"go-lint","files":["a.go"]},{"checker":"document","files":["b.md"]}]}
has_work=true
`

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		require.NoError(t, err)
		got, _ := os.ReadFile(ghOut)
		assert.Equal(t, expectedFile, string(got))
		assert.Equal(t, expectedFile, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("BASE_SHA 未設定 → stderr に must be set / exit 1", func(t *testing.T) {
		// Arrange
		// requireEnv が空文字も "must be set" として扱う (command.go の §requireEnv) ため、 空文字で env 違反を再現する。
		t.Setenv("BASE_SHA", "")
		t.Setenv("HEAD_SHA", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeOps{compareJSON: nil, diffOutput: nil, bucketResult: nil, fetchCalls: nil, bucketCalls: nil}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Equal(t, "BASE_SHA must be set\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("HEAD_SHA 未設定 → stderr に must be set / exit 1", func(t *testing.T) {
		// Arrange
		t.Setenv("BASE_SHA", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111")
		t.Setenv("HEAD_SHA", "")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeOps{compareJSON: nil, diffOutput: nil, bucketResult: nil, fetchCalls: nil, bucketCalls: nil}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Equal(t, "HEAD_SHA must be set\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("GITHUB_REPOSITORY 未設定 → stderr に must be set / exit 1", func(t *testing.T) {
		// Arrange
		t.Setenv("BASE_SHA", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111")
		t.Setenv("HEAD_SHA", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222")
		t.Setenv("GITHUB_REPOSITORY", "")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeOps{compareJSON: nil, diffOutput: nil, bucketResult: nil, fetchCalls: nil, bucketCalls: nil}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Equal(t, "GITHUB_REPOSITORY must be set\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("GH_TOKEN 未設定 → stderr に must be set / exit 1", func(t *testing.T) {
		// Arrange
		t.Setenv("BASE_SHA", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111")
		t.Setenv("HEAD_SHA", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "")
		op := &fakeOps{compareJSON: nil, diffOutput: nil, bucketResult: nil, fetchCalls: nil, bucketCalls: nil}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Equal(t, "GH_TOKEN must be set\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("compare API の merge_base SHA が 40 桁 hex でない → stderr に invalid merge_base / exit 1", func(t *testing.T) {
		// Arrange
		t.Setenv("BASE_SHA", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111")
		t.Setenv("HEAD_SHA", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeOps{
			compareJSON:  []byte(`{"merge_base_commit":{"sha":"deadbeef"},"ahead_by":0}`),
			diffOutput:   nil,
			bucketResult: nil,
			fetchCalls:   nil,
			bucketCalls:  nil,
		}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Equal(t, "ERROR: invalid merge_base SHA from compare API: deadbeef\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("compare API の ahead_by が負値 → stderr に invalid ahead_by / exit 1", func(t *testing.T) {
		// Arrange
		t.Setenv("BASE_SHA", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111")
		t.Setenv("HEAD_SHA", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeOps{
			compareJSON:  []byte(`{"merge_base_commit":{"sha":"cccc3333cccc3333cccc3333cccc3333cccc3333"},"ahead_by":-1}`),
			diffOutput:   nil,
			bucketResult: nil,
			fetchCalls:   nil,
			bucketCalls:  nil,
		}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Equal(t, "ERROR: invalid ahead_by from compare API: -1\n", stderr.String())
		assert.Empty(t, stdout.String())
	})

	t.Run("差分無 → files=[] / matrix={include:[]} / has_work=false", func(t *testing.T) {
		// Arrange
		t.Setenv("BASE_SHA", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111")
		t.Setenv("HEAD_SHA", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GH_TOKEN", "x")
		op := &fakeOps{
			compareJSON:  []byte(`{"merge_base_commit":{"sha":"cccc3333cccc3333cccc3333cccc3333cccc3333"},"ahead_by":0}`),
			diffOutput:   nil,
			bucketResult: nil,
			fetchCalls:   nil,
			bucketCalls:  nil,
		}
		ghOut := filepath.Join(t.TempDir(), "github_output")
		require.NoError(t, os.WriteFile(ghOut, nil, 0o600))
		var stdout, stderr bytes.Buffer
		const expectedFile = `files=[]
matrix={"include":[]}
has_work=false
`

		// Act
		err := doRun(t.Context(), &stdout, &stderr, ghOut, op)

		// Assert
		require.NoError(t, err)
		got, _ := os.ReadFile(ghOut)
		assert.Equal(t, expectedFile, string(got))
		assert.Empty(t, stderr.String())
	})
}

// TestRun は New() / run() wrapper を clix root から起動する経路の service-level test。 args 数違反が usage 出力 + exit 2 を返すことで、 doRun 直呼び test ではカバーされない `New` constructor と `run` 引数 check 経路を埋める (FLM_APP_0009 §endpoint の振る舞い決定 layer)。
func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("引数数違反 → stderr に usage / exit 2", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(New())
		fake := clix.NewFakeIO(t, []string{"detect"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", "usage: flame ci check detect <github_output_path>\n")
	})
}
