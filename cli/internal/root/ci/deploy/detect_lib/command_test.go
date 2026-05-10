package detect_lib_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/cli/internal/root/ci/deploy/detect_lib"
	"github.com/wakuwaku3/flame/lib/clix"
)

//nolint:paralleltest // t.Chdir で cwd を fake root に切り替えるため parallel 不可。
func TestRun(t *testing.T) {
	cases := []struct {
		setup          func(t *testing.T, root string)
		name           string
		expectedStdout string
		expectedFile   string
	}{
		{
			name:           "lib module 1 件のみ",
			setup:          singleLibFixture,
			expectedStdout: "matrix={\"libs\":[{\"module\":\"lib\",\"module_path\":\"lib\"}]}\nhas_libs=true\n",
			expectedFile:   "matrix={\"libs\":[{\"module\":\"lib\",\"module_path\":\"lib\"}]}\nhas_libs=true\n",
		},
		{
			name:           "lib / *_lib suffix 以外は除外",
			setup:          mixedSuffixFixture,
			expectedStdout: "matrix={\"libs\":[{\"module\":\"foo_lib\",\"module_path\":\"foo_lib\"},{\"module\":\"lib\",\"module_path\":\"lib\"}]}\nhas_libs=true\n",
			expectedFile:   "matrix={\"libs\":[{\"module\":\"foo_lib\",\"module_path\":\"foo_lib\"},{\"module\":\"lib\",\"module_path\":\"lib\"}]}\nhas_libs=true\n",
		},
		{
			name:           "配布対象 0 件なら libs=[]",
			setup:          noLibFixture,
			expectedStdout: "matrix={\"libs\":[]}\nhas_libs=false\n",
			expectedFile:   "matrix={\"libs\":[]}\nhas_libs=false\n",
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
			r.AddCommand(detect_lib.New())
			fake := clix.NewFakeIO(t, []string{"detect-lib", ghOut})

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
	r.AddCommand(detect_lib.New())
	fake := clix.NewFakeIO(t, []string{"detect-lib"})

	// Act
	err := r.Run(t.Context(), fake)

	// Assert
	code, ok := clix.ExitCodeOf(err)
	require.True(t, ok)
	assert.Equal(t, 2, code)
	fake.Verify(t, "", "usage: flame ci deploy detect-lib <github_output_path>\n")
}

func singleLibFixture(t *testing.T, root string) {
	t.Helper()
	mkModule(t, filepath.Join(root, "lib"))
}

func mixedSuffixFixture(t *testing.T, root string) {
	t.Helper()
	mkModule(t, filepath.Join(root, "lib"))
	mkModule(t, filepath.Join(root, "foo_lib"))
	mkModule(t, filepath.Join(root, "tool"))
	mkModule(t, filepath.Join(root, "service"))
}

func noLibFixture(t *testing.T, root string) {
	t.Helper()
	mkModule(t, filepath.Join(root, "tool"))
	mkModule(t, filepath.Join(root, "service"))
}

func mkModule(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, fsperm.Dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), fsperm.File))
}
