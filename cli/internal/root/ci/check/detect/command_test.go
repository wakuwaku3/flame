package detect

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
