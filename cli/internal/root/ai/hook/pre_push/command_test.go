package pre_push

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/lib/clix"
)

// service-level test (FLM_APP_0009): `flame ai hook pre-push` endpoint の入力空間を主要分岐 9 ケースで覆う。 git fixture を t.TempDir() に組み、 upstream tracking ref を local refs/remotes/origin/main で fake する (実 push を行わずに upstream...HEAD 差分を作るため)。
//
//nolint:paralleltest // hookは os.Getenv / state file 等の process global 副作用を持つため parallel 不可
func TestRun(t *testing.T) {
	t.Run("tool_name が Bash 以外なら素通り", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Edit","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("Bash だが git push 以外は素通り", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"ls -al"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("git push --dry-run は素通り", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push --dry-run origin main"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("git push --delete は素通り", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push origin --delete branch"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("git push が他コマンドと chain されると block (chain reason)", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git commit -m foo && git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, expectedBlockJSON(t, chainBlockReason()), "")
	})

	t.Run("upstream / origin/main が無ければ素通り", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("upstream あり / 差分なしは素通り", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("差分有 (ADR 含まず) → block reason に rule-adr-sync-reviewer 含まない", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		commitFile(t, repo, "lib/x.go", "package x\n", "add x")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		bullet := formatBulletList([]string{"lib/x.go"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithoutADR(bullet, bullet, nil)), "")
	})

	t.Run("差分有 (ADR 含む) → block reason に rule-adr-sync-reviewer 含む", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		commitFile(t, repo, "docs/adr/general/FLM_GEN_0099__sample.md", "# sample\n", "add adr")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		bullet := formatBulletList([]string{"docs/adr/general/FLM_GEN_0099__sample.md"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithADR(bullet, bullet, nil)), "")
	})

	t.Run("hook input の cwd が指す repo の HEAD を見る (CLAUDE_PROJECT_DIR は fallback のみ)", func(t *testing.T) {
		// Arrange
		// CLAUDE_PROJECT_DIR は差分なし repo、 cwd は差分あり repo を指す。 cwd 経路が採用されれば cwd の diff (lib/x.go) で block されるはず。 fallback (= CLAUDE_PROJECT_DIR) を見ると差分 0 で素通りしてしまうため、 stdout に block JSON が出ること自体が cwd 経路採用の証拠になる。
		mainRepo := setupRepo(t)
		fakeUpstream(t, mainRepo)
		worktreeRepo := setupRepo(t)
		fakeUpstream(t, worktreeRepo)
		commitFile(t, worktreeRepo, "lib/x.go", "package x\n", "add x")
		t.Setenv("CLAUDE_PROJECT_DIR", mainRepo)
		fake := newFakeIO(t)
		stdin, err := json.Marshal(map[string]any{
			"tool_name":  "Bash",
			"tool_input": map[string]string{"command": "git push"},
			"cwd":        worktreeRepo,
		})
		require.NoError(t, err)
		fake.SetStdin(t, strings.NewReader(string(stdin)))

		// Act
		runErr := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, runErr)
		bullet := formatBulletList([]string{"lib/x.go"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithoutADR(bullet, bullet, nil)), "")
	})

	t.Run("cwd が git repo 外なら副作用なしで素通り", func(t *testing.T) {
		// Arrange
		nonGitDir := t.TempDir()
		t.Setenv("CLAUDE_PROJECT_DIR", "")
		fake := newFakeIO(t)
		stdin, err := json.Marshal(map[string]any{
			"tool_name":  "Bash",
			"tool_input": map[string]string{"command": "git push"},
			"cwd":        nonGitDir,
		})
		require.NoError(t, err)
		fake.SetStdin(t, strings.NewReader(string(stdin)))

		// Act
		runErr := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, runErr)
		fake.Verify(t, "", "")
	})

	t.Run("同 fire を 2 回続けると 2 回目は state 一致で素通り", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		commitFile(t, repo, "lib/x.go", "package x\n", "add x")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake1 := newFakeIO(t)
		fake1.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))
		require.NoError(t, newRoot().Run(t.Context(), fake1))
		require.NotEmpty(t, fake1.StdoutString(t))
		fake2 := newFakeIO(t)
		fake2.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake2)

		// Assert
		require.NoError(t, err)
		fake2.Verify(t, "", "")
	})

	t.Run("flame.yaml の stage1_extra_agents が block reason の段階 1 list と件数に反映される (ADR 含まず)", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		writeFlameYAML(t, repo, "flame:\n  ai:\n    pre_push:\n      stage1_extra_agents:\n        - foo\n        - bar\n")
		commitFile(t, repo, "lib/x.go", "package x\n", "add x")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		bullet := formatBulletList([]string{"lib/x.go"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithoutADR(bullet, bullet, []string{"foo", "bar"})), "")
	})

	t.Run("flame.yaml の stage1_extra_agents が block reason の段階 1 list と件数に反映される (ADR 含む)", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		writeFlameYAML(t, repo, "flame:\n  ai:\n    pre_push:\n      stage1_extra_agents:\n        - foo\n")
		commitFile(t, repo, "docs/adr/general/FLM_GEN_0099__sample.md", "# sample\n", "add adr")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		bullet := formatBulletList([]string{"docs/adr/general/FLM_GEN_0099__sample.md"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithADR(bullet, bullet, []string{"foo"})), "")
	})

	t.Run("flame.yaml の stage1_extra_agents は先勝ち de-dup される", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		writeFlameYAML(t, repo, "flame:\n  ai:\n    pre_push:\n      stage1_extra_agents:\n        - foo\n        - foo\n        - bar\n")
		commitFile(t, repo, "lib/x.go", "package x\n", "add x")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		bullet := formatBulletList([]string{"lib/x.go"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithoutADR(bullet, bullet, []string{"foo", "bar"})), "")
	})

	t.Run("flame.yaml が空 list なら現状の reason text と等価", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		writeFlameYAML(t, repo, "flame:\n  ai:\n    pre_push:\n      stage1_extra_agents: []\n")
		commitFile(t, repo, "lib/x.go", "package x\n", "add x")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		bullet := formatBulletList([]string{"lib/x.go"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithoutADR(bullet, bullet, nil)), "")
	})

	t.Run("flame.yaml が parse error なら extra agents 無効として現状 reason に fallback", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		fakeUpstream(t, repo)
		writeFlameYAML(t, repo, "this: is: not valid yaml :::\n")
		commitFile(t, repo, "lib/x.go", "package x\n", "add x")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake := newFakeIO(t)
		fake.SetStdin(t, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`))

		// Act
		err := newRoot().Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		bullet := formatBulletList([]string{"lib/x.go"})
		fake.Verify(t, expectedBlockJSON(t, blockReasonWithoutADR(bullet, bullet, nil)), "")
	})
}

func writeFlameYAML(tb testing.TB, repo, content string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(filepath.Join(repo, "flame.yaml"), []byte(content), 0o600))
}

// setupRepo は fake repo を t.TempDir() に作り、 git init + 初回 commit を打つ。 hook は git ls-files / git diff / git rev-parse を呼ぶため、 commit の有る repo が前提。 .claude/.cache/ は hook 自身の state を入れる場所のため untracked 走査から除外する .gitignore を予めコミットしておく (実 repo では root .gitignore に同等エントリが入っている)。
func setupRepo(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	mustGit(tb, root, "init", "-q", "-b", "main")
	mustGit(tb, root, "config", "user.email", "test@example.com")
	mustGit(tb, root, "config", "user.name", "test")
	require.NoError(tb, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".claude/.cache/\n"), 0o600))
	require.NoError(tb, os.WriteFile(filepath.Join(root, "README.md"), []byte("seed\n"), 0o600))
	mustGit(tb, root, "add", ".gitignore", "README.md")
	mustGit(tb, root, "commit", "-q", "-m", "init")
	return root
}

// fakeUpstream は実 push を行わずに upstream tracking ref を擬似的に作る。 現 HEAD を refs/remotes/origin/main に複写したうえで、 git config 直書きで branch.main.{remote,merge} を設定する (git branch --set-upstream-to は upstream を「branch」として要求し、 update-ref で作った remote-tracking ref を受け付けないため config 直書きで回避)。 hook は `git rev-parse @{u}` で origin/main を解決し、 `git diff @{u}...HEAD` が空 (= 差分なし) になる初期状態を作る。 caller が後続で commit を積むとそれが diff として現れる。
func fakeUpstream(tb testing.TB, repo string) {
	tb.Helper()
	mustGit(tb, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	mustGit(tb, repo, "config", "branch.main.remote", "origin")
	mustGit(tb, repo, "config", "branch.main.merge", "refs/heads/main")
}

func commitFile(tb testing.TB, repo, rel, body, msg string) {
	tb.Helper()
	full := filepath.Join(repo, rel)
	require.NoError(tb, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(tb, os.WriteFile(full, []byte(body), 0o600))
	mustGit(tb, repo, "add", rel)
	mustGit(tb, repo, "commit", "-q", "-m", msg)
}

func mustGit(tb testing.TB, repo string, args ...string) {
	tb.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // G204: test fixture: 各 args は test 内 caller 内部値で外部入力ではない。
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	require.NoErrorf(tb, err, "git %s: %s", strings.Join(args, " "), string(out))
}

func newRoot() interface {
	AddCommand(clix.Subcommand)
	Run(ctx context.Context, cio clix.IO) error
} {
	r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
	r.AddCommand(New())
	return r
}

func newFakeIO(tb testing.TB) *clix.FakeIO {
	tb.Helper()
	return clix.NewFakeIO(tb, []string{"pre-push"})
}

// expectedBlockJSON は emitBlock が出力する block decision JSON を test 側で再現する。 §assertion 規約 (FLM_APP_0009) に従い expected を Marshal して actual と完全一致比較するため、 production と同じ encoder 設定 (SetEscapeHTML(false) / SetIndent / Encode の末尾改行) で組む。
func expectedBlockJSON(tb testing.TB, reason string) string {
	tb.Helper()
	payload := struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{Decision: "block", Reason: reason}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	require.NoError(tb, enc.Encode(&payload))
	return buf.String()
}
