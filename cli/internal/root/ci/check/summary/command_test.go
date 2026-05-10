package summary_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/ci/check/summary"
	"github.com/wakuwaku3/flame/lib/clix"
)

type sumCase struct {
	name           string
	bucket         string
	check          string
	noop           string
	label          string
	expectedStdout string
	expectedStderr string
	expectedCode   int
}

func sumCases() []sumCase {
	return []sumCase{
		{
			name:   "bucket success / check success / noop skipped / label success",
			bucket: "success", check: "success", noop: "skipped", label: "success",
			expectedStdout: "OK: bucket=success check=success noop=skipped label=success\n",
			expectedStderr: "",
			expectedCode:   0,
		},
		{
			name:   "bucket success / check skipped / noop success / label success",
			bucket: "success", check: "skipped", noop: "success", label: "success",
			expectedStdout: "OK: bucket=success check=skipped noop=success label=success\n",
			expectedStderr: "",
			expectedCode:   0,
		},
		{
			name:   "bucket success / check success / noop skipped / label skipped (fork PR)",
			bucket: "success", check: "success", noop: "skipped", label: "skipped",
			expectedStdout: "OK: bucket=success check=success noop=skipped label=skipped\n",
			expectedStderr: "",
			expectedCode:   0,
		},
		{
			name:   "bucket failure → bucket job did not succeed",
			bucket: "failure", check: "success", noop: "skipped", label: "success",
			expectedStdout: "",
			expectedStderr: "bucket job did not succeed (result=failure)\n",
			expectedCode:   1,
		},
		{
			name:   "check failure → CHECK job result was 'failure'",
			bucket: "success", check: "failure", noop: "skipped", label: "success",
			expectedStdout: "",
			expectedStderr: "CHECK job result was 'failure' (expected success or skipped)\n",
			expectedCode:   1,
		},
		{
			name:   "noop canceled → NOOP job result was 'canceled'",
			bucket: "success", check: "skipped", noop: "canceled", label: "success",
			expectedStdout: "",
			expectedStderr: "NOOP job result was 'canceled' (expected success or skipped)\n",
			expectedCode:   1,
		},
		{
			name:   "label failure → LABEL job result was 'failure'",
			bucket: "success", check: "success", noop: "skipped", label: "failure",
			expectedStdout: "",
			expectedStderr: "LABEL job result was 'failure' (expected success or skipped)\n",
			expectedCode:   1,
		},
		{
			name:   "check skipped かつ noop skipped → neither check nor noop succeeded",
			bucket: "success", check: "skipped", noop: "skipped", label: "success",
			expectedStdout: "",
			expectedStderr: "neither check nor noop succeeded\n",
			expectedCode:   1,
		},
	}
}

func TestRun(t *testing.T) {
	for _, tc := range sumCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			t.Setenv("BUCKET_RESULT", tc.bucket)
			t.Setenv("CHECK_RESULT", tc.check)
			t.Setenv("NOOP_RESULT", tc.noop)
			t.Setenv("LABEL_RESULT", tc.label)
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
				require.True(t, ok)
				assert.Equal(t, tc.expectedCode, code)
			}
			fake.Verify(t, tc.expectedStdout, tc.expectedStderr)
		})
	}
}
