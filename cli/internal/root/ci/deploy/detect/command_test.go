package detect_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/deploy/detect"
	"github.com/wakuwaku3/flame/lib/clix"
)

//nolint:paralleltest // t.Chdir で cwd を fake root に切り替えるため parallel 不可。
func TestRun(t *testing.T) {
	cases := []struct {
		setup          func(t *testing.T, root string)
		name           string
		expectedStdout string
		expectedFile   string
		expectedCode   int
	}{
		{
			name:           "_tool suffix を持つ app を 1 件 enumerate する",
			setup:          singleToolFixture,
			expectedStdout: "matrix={\"apps\":[{\"app_name\":\"flame\",\"module\":\"cli\",\"app_dir\":\"flame_tool\"}]}\nhas_apps=true\n",
			expectedFile:   "matrix={\"apps\":[{\"app_name\":\"flame\",\"module\":\"cli\",\"app_dir\":\"flame_tool\"}]}\nhas_apps=true\n",
			expectedCode:   0,
		},
		{
			name:           "_tool suffix が無い app は除外する",
			setup:          noToolFixture,
			expectedStdout: "matrix={\"apps\":[]}\nhas_apps=false\n",
			expectedFile:   "matrix={\"apps\":[]}\nhas_apps=false\n",
			expectedCode:   0,
		},
		{
			name:           "module を辞書順に走査し _tool app を辞書順に並べる",
			setup:          multiModuleFixture,
			expectedStdout: "matrix={\"apps\":[{\"app_name\":\"a\",\"module\":\"alpha\",\"app_dir\":\"a_tool\"},{\"app_name\":\"b\",\"module\":\"beta\",\"app_dir\":\"b_tool\"},{\"app_name\":\"c\",\"module\":\"beta\",\"app_dir\":\"c_tool\"}]}\nhas_apps=true\n",
			expectedFile:   "matrix={\"apps\":[{\"app_name\":\"a\",\"module\":\"alpha\",\"app_dir\":\"a_tool\"},{\"app_name\":\"b\",\"module\":\"beta\",\"app_dir\":\"b_tool\"},{\"app_name\":\"c\",\"module\":\"beta\",\"app_dir\":\"c_tool\"}]}\nhas_apps=true\n",
			expectedCode:   0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			fakeRoot := t.TempDir()
			tc.setup(t, fakeRoot)
			ghOut := filepath.Join(t.TempDir(), "github_output")
			require.NoError(t, os.WriteFile(ghOut, nil, fsperm.File))
			t.Chdir(fakeRoot)
			r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
			r.AddCommand(detect.New())
			fake := clix.NewFakeIO(t, []string{"detect", ghOut})

			// Act
			err := r.Run(t.Context(), fake)

			// Assert
			require.NoError(t, err)
			fake.Verify(t, tc.expectedStdout, "")
			gotFile, readErr := os.ReadFile(ghOut)
			require.NoError(t, readErr)
			assert.Equal(t, tc.expectedFile, string(gotFile))
		})
	}
}

func TestRunUsage(t *testing.T) {
	t.Parallel()

	// Arrange
	r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
	r.AddCommand(detect.New())
	fake := clix.NewFakeIO(t, []string{"detect"})

	// Act
	err := r.Run(t.Context(), fake)

	// Assert
	code, ok := clix.ExitCodeOf(err)
	require.True(t, ok)
	assert.Equal(t, 2, code)
	fake.Verify(t, "", "usage: flame ci deploy detect <github_output_path>\n")
}

// 各 fixture function は test 内 helper として t.Helper() を呼ぶ (FLM_APP_0009 §test helper signature)。
func singleToolFixture(t *testing.T, root string) {
	t.Helper()
	mkModule(t, filepath.Join(root, "cli"))
	mkAppDir(t, filepath.Join(root, "cli", "cmd", "flame_tool"))
}

func noToolFixture(t *testing.T, root string) {
	t.Helper()
	mkModule(t, filepath.Join(root, "cli"))
	mkAppDir(t, filepath.Join(root, "cli", "cmd", "internal_helper"))
}

func multiModuleFixture(t *testing.T, root string) {
	t.Helper()
	mkModule(t, filepath.Join(root, "alpha"))
	mkAppDir(t, filepath.Join(root, "alpha", "cmd", "a_tool"))
	mkAppDir(t, filepath.Join(root, "alpha", "cmd", "non_tool_helper"))
	mkModule(t, filepath.Join(root, "beta"))
	mkAppDir(t, filepath.Join(root, "beta", "cmd", "c_tool"))
	mkAppDir(t, filepath.Join(root, "beta", "cmd", "b_tool"))
	mkModule(t, filepath.Join(root, "docs"))
	require.NoError(t, os.Remove(filepath.Join(root, "docs", "go.mod")))
}

func mkModule(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, fsperm.Dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), fsperm.File))
}

func mkAppDir(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, fsperm.Dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), fsperm.File))
}
