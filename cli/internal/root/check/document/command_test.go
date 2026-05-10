package document_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/document"
	"github.com/wakuwaku3/flame/lib/clix"
)

// TestRun は各サブテスト内で t.Setenv / t.Chdir (FLAME_CHECKER_MODE / cwd 制御) を呼ぶため t.Parallel を採用しない。 testing package が当該 helper と t.Parallel の併用を禁止している (Go testing: "test using t.Setenv or t.Chdir can not use t.Parallel")。 サブテスト間の独立性は t.TempDir で確保する。
func TestRun(t *testing.T) {
	t.Run("引数指定で全 intra-repo link が解決すれば success", func(t *testing.T) {
		// t.Parallel は省く: 本ケース内で t.Setenv / t.Chdir を呼ぶため、 Go の testing が parallel との併用を禁じている。

		// Arrange
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "target.md"), "# target\n")
		mdPath := filepath.Join(dir, "doc.md")
		writeFile(t, mdPath, "# doc\n\nsee [target](target.md).\n")
		t.Chdir(dir)
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")
		setupFakeMarkdownlint(t, fakeMarkdownlintSuccess)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(document.New())
		fake := clix.NewFakeIO(t, []string{"document", mdPath})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, fakeMarkdownlintStdout, fakeMarkdownlintStderr)
	})

	t.Run("intra-repo の broken link を検出して FAIL 行を出し exit 1", func(t *testing.T) {
		// t.Parallel は省く: 本ケース内で t.Setenv / t.Chdir を呼ぶため、 Go の testing が parallel との併用を禁じている。

		// Arrange
		dir := t.TempDir()
		mdPath := filepath.Join(dir, "doc.md")
		writeFile(t, mdPath, "# doc\n\nsee [missing](missing.md).\n")
		t.Chdir(dir)
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")
		setupFakeMarkdownlint(t, fakeMarkdownlintSuccess)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(document.New())
		fake := clix.NewFakeIO(t, []string{"document", mdPath})
		expectedStderr := fakeMarkdownlintStderr + "FAIL: " + mdPath + ": broken link: missing.md\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok, "error must carry exit code")
		assert.Equal(t, 1, code)
		fake.Verify(t, fakeMarkdownlintStdout, expectedStderr)
	})

	t.Run("コードフェンス / inline code 内 link と URL スキーム / anchor / scheme-rel は intra-repo 検査から除外する", func(t *testing.T) {
		// t.Parallel は省く: 本ケース内で t.Setenv / t.Chdir を呼ぶため、 Go の testing が parallel との併用を禁じている。

		// Arrange
		dir := t.TempDir()
		mdPath := filepath.Join(dir, "doc.md")
		body := "# doc\n\n" +
			"```\n" +
			"[fenced](missing-fenced.md)\n" +
			"```\n\n" +
			"`[inline](missing-inline.md)` text\n\n" +
			"[ext](https://example.com/x.md) [mail](mailto:a@b) [anchor](#section) [scheme-rel](//example.com/x)\n"
		writeFile(t, mdPath, body)
		t.Chdir(dir)
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")
		setupFakeMarkdownlint(t, fakeMarkdownlintSuccess)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(document.New())
		fake := clix.NewFakeIO(t, []string{"document", mdPath})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, fakeMarkdownlintStdout, fakeMarkdownlintStderr)
	})

	t.Run("markdownlint-cli2 が non-zero 終了すると検査失敗で exit 1 を伝搬し cli2 自身の出力は素通しする", func(t *testing.T) {
		// t.Parallel は省く: 本ケース内で t.Setenv / t.Chdir を呼ぶため、 Go の testing が parallel との併用を禁じている。

		// Arrange
		dir := t.TempDir()
		mdPath := filepath.Join(dir, "doc.md")
		writeFile(t, mdPath, "# doc\n")
		t.Chdir(dir)
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")
		setupFakeMarkdownlint(t, fakeMarkdownlintFailure)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(document.New())
		fake := clix.NewFakeIO(t, []string{"document", mdPath})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, fakeMarkdownlintStdout, fakeMarkdownlintStderr)
	})

	t.Run("FLAME_CHECKER_MODE 不正値は usage 違反で exit 2", func(t *testing.T) {
		// t.Parallel は省く: 本ケース内で t.Setenv / t.Chdir を呼ぶため、 Go の testing が parallel との併用を禁じている。

		// Arrange
		dir := t.TempDir()
		mdPath := filepath.Join(dir, "doc.md")
		writeFile(t, mdPath, "# doc\n")
		t.Chdir(dir)
		t.Setenv("FLAME_CHECKER_MODE", "invalid")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(document.New())
		fake := clix.NewFakeIO(t, []string{"document", mdPath})
		const expectedStderr = "error: invalid FLAME_CHECKER_MODE='invalid' (expected 'fix' or 'diagnose')\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("引数なし起動は git ls-files 経路で対象 0 件なら no-op success", func(t *testing.T) {
		// t.Parallel は省く: 本ケース内で t.Setenv / t.Chdir を呼ぶため、 Go の testing が parallel との併用を禁じている。

		// Arrange
		dir := t.TempDir()
		runGit(t.Context(), t, dir, "init", "-q")
		runGit(t.Context(), t, dir, "config", "user.email", "test@example.com")
		runGit(t.Context(), t, dir, "config", "user.name", "test")
		t.Chdir(dir)
		t.Setenv("FLAME_CHECKER_MODE", "diagnose")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(document.New())
		fake := clix.NewFakeIO(t, []string{"document"})
		const expectedStdout = "no Markdown files to lint\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, expectedStdout, "")
	})
}

const (
	fakeMarkdownlintSuccess = 0
	fakeMarkdownlintFailure = 1
	fakeMarkdownlintStdout  = "fake-out\n"
	fakeMarkdownlintStderr  = "fake-err\n"
)

// setupFakeMarkdownlint は markdownlint-cli2 を test 内で fake binary に置き換える。 service-level test (FLM_APP_0009) で外部 process 起動経路の挙動を fake (= 本物に近い動作の軽量実装) で制御するための仕組み。 production が exec 経由で呼び出す経路は維持しつつ、 PATH 先頭に shebang script を挿入することで cli2 の出力 / exit code を test 側から決定論的に再現する。
func setupFakeMarkdownlint(tb testing.TB, exitCode int) {
	tb.Helper()
	binDir := tb.TempDir()
	script := "#!/usr/bin/env bash\n" +
		"echo fake-out\n" +
		"echo fake-err >&2\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	binPath := filepath.Join(binDir, "markdownlint-cli2")
	require.NoError(tb, os.WriteFile(binPath, []byte(script), 0o600))
	// fake binary は exec 起動経路を成立させるため実行ビット必須。 Chmod は test 専用 TempDir 配下の fake binary に対する操作で、 production の file permission ポリシーには影響しない。 gosec G302 は test fixture の意図性まで判定できないため局所抑制する (FLM_GEN_0006)。
	require.NoError(tb, os.Chmod(binPath, 0o700)) //nolint:gosec // G302: test 専用 TempDir 配下の fake binary に exec ビットを付与する
	tb.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeFile(tb testing.TB, path, body string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(path, []byte(body), 0o600))
}

func runGit(ctx context.Context, tb testing.TB, dir string, args ...string) {
	tb.Helper()
	// 起動コマンドは固定文字列 "git"、 args は本 test ファイル内の caller から渡される定数列で外部入力でない。 gosec G204 は変数引数の検出のみで test fixture 起動の意図性まで判定できないため局所抑制する (FLM_GEN_0006)。
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: test fixture 用の固定 git subcommand 起動
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(tb, err, "git %s failed: %s", strings.Join(args, " "), string(out))
}
