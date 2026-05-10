package shell_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/shell"
	"github.com/wakuwaku3/flame/lib/clix"
)

// service-level test は shellcheck 外部バイナリに依存する。 PATH に存在しない環境では skip し、 devbox 配下 / CI など PATH に shellcheck がある環境でのみ検査する (FLM_APP_0009 §service-level test)。
func requireShellcheck(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skipf("shellcheck not found in PATH: %v", err)
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("全て妥当な shell ファイルなら no-op success", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// Arrange
		dir := t.TempDir()
		ok := filepath.Join(dir, "ok.sh")
		require.NoError(t, os.WriteFile(ok, []byte("#!/usr/bin/env bash\nset -euo pipefail\nlower_var=\"hello\"\necho \"$lower_var\"\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", ok})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("ファイル名違反は FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// Arrange
		dir := t.TempDir()
		bad := filepath.Join(dir, "Bad_Name.sh")
		require.NoError(t, os.WriteFile(bad, []byte("#!/usr/bin/env bash\nset -euo pipefail\necho ok\n"), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", bad})
		expectedStderr := "FAIL: " + bad + ": filename must be kebab-case with .sh extension (got: Bad_Name.sh)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run(".github/workflows/tests/ 直下の wf__/trg__ 系 snake_case は許容する (FLM_APP_0002 §配置・命名 例外)", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// FLM_APP_0002 §配置・命名 で `.github/workflows/tests/<basename>.sh` は
		// 対応 workflow basename (snake_case + `__` 区切り) を継承する例外規定
		// が定められている。 当該 dir 直下の wf__/trg__ 系 snake_case ファイル名が
		// kebab-case 強制で FAIL しないことを behavior レベルで固定する。
		//
		// Arrange
		dir := t.TempDir()
		testsDir := filepath.Join(dir, ".github", "workflows", "tests")
		require.NoError(t, os.MkdirAll(testsDir, 0o750))
		body := "#!/usr/bin/env bash\nset -euo pipefail\nlower_var=\"hello\"\necho \"$lower_var\"\n"
		paths := []string{
			filepath.Join(testsDir, "wf__check.sh"),
			filepath.Join(testsDir, "wf__check__diff.sh"),
			filepath.Join(testsDir, "wf__deploy_lib.sh"),
			filepath.Join(testsDir, "trg__push__main.sh"),
			filepath.Join(testsDir, "trg__pull_request__opened_synchronize_reopened.sh"),
		}
		for _, p := range paths {
			require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		}
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, append([]string{"shell"}, paths...))

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run(".github/workflows/tests/ 直下の kebab-case (= 例外対象外の helper) も許容する", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// 例外規定は「workflow basename を継承する場合に限り snake_case を許容」 する
		// 形で kebab-case を排除しない。 `.github/workflows/tests/` 直下の
		// helper (= workflow に対応しない補助 .sh) も kebab-case であれば通る。
		//
		// Arrange
		dir := t.TempDir()
		testsDir := filepath.Join(dir, ".github", "workflows", "tests")
		require.NoError(t, os.MkdirAll(testsDir, 0o750))
		target := filepath.Join(testsDir, "helper.sh")
		body := "#!/usr/bin/env bash\nset -euo pipefail\nlower_var=\"hello\"\necho \"$lower_var\"\n"
		require.NoError(t, os.WriteFile(target, []byte(body), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", target})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run(".github/workflows/tests/shared/ 配下は例外対象外で kebab-case 強制", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// FLM_APP_0002 §配置・命名 は shared/ subdir を「例外の対象外」 と明示。
		// shared/ 配下の snake_case ファイルは workflow basename ではないため
		// kebab-case 強制で FAIL する。
		//
		// Arrange
		dir := t.TempDir()
		sharedDir := filepath.Join(dir, ".github", "workflows", "tests", "shared")
		require.NoError(t, os.MkdirAll(sharedDir, 0o750))
		target := filepath.Join(sharedDir, "foo_bar.sh")
		body := "#!/usr/bin/env bash\nset -euo pipefail\nlower_var=\"hello\"\necho \"$lower_var\"\n"
		require.NoError(t, os.WriteFile(target, []byte(body), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", target})
		expectedStderr := "FAIL: " + target + ": filename must be kebab-case with .sh extension (got: foo_bar.sh)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run(".github/workflows/tests/ 直下でも workflow basename にも kebab にも合致しないファイルは FAIL する", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// 例外 dir 内で snake_case だが (trg|wf)__ prefix を持たない
		// `bad_name.sh` 等は、 例外規定にも kebab-case にも合致せず FAIL する。
		// FAIL message は dir 文脈に応じて「kebab-case か workflow basename」 の
		// 両許容を明示する形に切り替わる。
		//
		// Arrange
		dir := t.TempDir()
		testsDir := filepath.Join(dir, ".github", "workflows", "tests")
		require.NoError(t, os.MkdirAll(testsDir, 0o750))
		target := filepath.Join(testsDir, "bad_name.sh")
		body := "#!/usr/bin/env bash\nset -euo pipefail\nlower_var=\"hello\"\necho \"$lower_var\"\n"
		require.NoError(t, os.WriteFile(target, []byte(body), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", target})
		expectedStderr := "FAIL: " + target + ": filename must be kebab-case or inherit corresponding workflow basename (snake_case + '__' separators) with .sh extension (FLM_APP_0002) (got: bad_name.sh)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("UPPER_SNAKE_CASE な local 変数は FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// Arrange
		dir := t.TempDir()
		target := filepath.Join(dir, "upper-local.sh")
		body := strings.Join([]string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"FOO_BAR=\"value\"",
			"echo \"$FOO_BAR\"",
			"",
		}, "\n")
		require.NoError(t, os.WriteFile(target, []byte(body), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", target})
		expectedStderr := "FAIL: " + target + ":3: 'FOO_BAR' is UPPER_SNAKE_CASE; non-exported locals must be lower_snake_case (FLM_APP_0002)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("allowlist の環境変数 (PATH 等) は UPPER_SNAKE_CASE でも許容する", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// Arrange
		dir := t.TempDir()
		target := filepath.Join(dir, "allow-list.sh")
		body := strings.Join([]string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"IFS=$'\\n'",
			"echo \"$PATH\"",
			"",
		}, "\n")
		require.NoError(t, os.WriteFile(target, []byte(body), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", target})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("export / local prefix が付いた UPPER_SNAKE_CASE は許容する", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// Arrange
		dir := t.TempDir()
		target := filepath.Join(dir, "exported-var.sh")
		body := strings.Join([]string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"export MY_VAR=\"value\"",
			"echo \"$MY_VAR\"",
			"",
		}, "\n")
		require.NoError(t, os.WriteFile(target, []byte(body), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", target})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("shellcheck 違反は子プロセスの出力を伝搬し exit code 1 を返す", func(t *testing.T) {
		t.Parallel()
		requireShellcheck(t)

		// Arrange
		dir := t.TempDir()
		target := filepath.Join(dir, "shellcheck-violation.sh")
		// SC2086 (未引用展開) を意図的に発生させる。
		body := strings.Join([]string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"foo=$1",
			"echo $foo",
			"",
		}, "\n")
		require.NoError(t, os.WriteFile(target, []byte(body), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell", target})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
	})

	t.Run("引数なし起動は usage を stderr に出して exit code 2 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(shell.New())
		fake := clix.NewFakeIO(t, []string{"shell"})
		const expectedStderr = "usage: flame check shell <shell_file>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})
}
