package configure_go_auth_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/ci/configure_go_auth"
	"github.com/wakuwaku3/flame/lib/clix"
)

// service-level test (FLM_APP_0009): `flame ci configure-go-auth` は GH_TOKEN env を使って `git config --global url.<token>.insteadOf https://github.com/` を冪等に書き込む単機能 endpoint。 副作用が global git config なので、 test では HOME を TempDir に切り替えて隔離した上で実 git に書かせる (FLM_APP_0009 §mock を採用しない)。
//
//nolint:paralleltest // env 操作 (HOME / GH_TOKEN) が process global 副作用を持つため parallel 不可
func TestRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Run("GH_TOKEN env が無いと failure (exit 1)", func(t *testing.T) {
		// Arrange
		t.Setenv("HOME", t.TempDir())
		t.Setenv("GH_TOKEN", "")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(configure_go_auth.New())
		fake := clix.NewFakeIO(t, []string{"configure-go-auth"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), "GH_TOKEN env must be set")
	})

	t.Run("引数があると usage 失敗 (exit 2)", func(t *testing.T) {
		// Arrange
		t.Setenv("HOME", t.TempDir())
		t.Setenv("GH_TOKEN", "x")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(configure_go_auth.New())
		fake := clix.NewFakeIO(t, []string{"configure-go-auth", "extra"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		assert.Contains(t, fake.StderrString(t), "usage: flame ci configure-go-auth")
	})

	t.Run("GH_TOKEN を URL rewrite として global gitconfig に注入する", func(t *testing.T) {
		// Arrange
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", home)
		t.Setenv("GH_TOKEN", "test-token-xyz")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(configure_go_auth.New())
		fake := clix.NewFakeIO(t, []string{"configure-go-auth"})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		// global gitconfig (= HOME/.gitconfig) に書き込まれた事を verify する。
		gitConfig := filepath.Join(home, ".gitconfig")
		out, readErr := exec.CommandContext(t.Context(), "git", "config", "--file", gitConfig, "--list").Output() //nolint:gosec // test fixture: gitConfig は test 内 TempDir 配下の固定 path。
		require.NoError(t, readErr)
		text := string(out)
		assert.True(t, strings.Contains(text, "x-access-token:test-token-xyz@github.com") || strings.Contains(text, "url.https://x-access-token:test-token-xyz"), "expected URL rewrite in gitconfig:\n%s", text)
	})
}
