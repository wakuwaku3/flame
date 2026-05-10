package path_base

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
}
