package summary_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/ci/deploy/summary"
	"github.com/wakuwaku3/flame/lib/clix"
)

func TestRun(t *testing.T) {
	cases := []struct {
		name           string
		enumerate      string
		release        string
		noop           string
		expectedStdout string
		expectedStderr string
		expectedCode   int
	}{
		{
			name:           "全 success なら ok 行を stdout に出して exit 0",
			enumerate:      "success",
			release:        "success",
			noop:           "success",
			expectedStdout: "deploy: ok (enumerate=success release=success noop=success)\n",
			expectedStderr: "",
			expectedCode:   0,
		},
		{
			name:           "release が skipped なら ok 扱い",
			enumerate:      "success",
			release:        "skipped",
			noop:           "success",
			expectedStdout: "deploy: ok (enumerate=success release=skipped noop=success)\n",
			expectedStderr: "",
			expectedCode:   0,
		},
		{
			name:           "noop が skipped なら ok 扱い",
			enumerate:      "success",
			release:        "success",
			noop:           "skipped",
			expectedStdout: "deploy: ok (enumerate=success release=success noop=skipped)\n",
			expectedStderr: "",
			expectedCode:   0,
		},
		{
			name:           "enumerate が failure なら failure 行を stderr に出して exit 1",
			enumerate:      "failure",
			release:        "success",
			noop:           "success",
			expectedStdout: "",
			expectedStderr: "deploy: failure (enumerate=failure release=success noop=success)\n",
			expectedCode:   1,
		},
		{
			name:           "release が failure なら failure 行を stderr に出して exit 1",
			enumerate:      "success",
			release:        "failure",
			noop:           "success",
			expectedStdout: "",
			expectedStderr: "deploy: failure (enumerate=success release=failure noop=success)\n",
			expectedCode:   1,
		},
		{
			name:           "noop が canceled なら failure 行を stderr に出して exit 1",
			enumerate:      "success",
			release:        "success",
			noop:           "canceled",
			expectedStdout: "",
			expectedStderr: "deploy: failure (enumerate=success release=success noop=canceled)\n",
			expectedCode:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			t.Setenv("ENUMERATE_RESULT", tc.enumerate)
			t.Setenv("RELEASE_RESULT", tc.release)
			t.Setenv("NOOP_RESULT", tc.noop)
			r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
			r.AddCommand(summary.New())
			fake := clix.NewFakeIO(t, []string{"summary"})

			// Act
			err := r.Run(t.Context(), fake)

			// Assert
			if tc.expectedCode == 0 {
				require.NoError(t, err)
			} else {
				code, ok := clix.ExitCodeOf(err)
				require.True(t, ok, "error must carry exit code")
				assert.Equal(t, tc.expectedCode, code)
			}
			fake.Verify(t, tc.expectedStdout, tc.expectedStderr)
		})
	}
}
