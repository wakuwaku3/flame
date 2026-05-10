package stop

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/lib/clix"
)

// service-level test (FLM_APP_0009): `flame ai hook stop` endpoint の入力空間 (agent_id 含 stdin / stop_hook_active 含 stdin / state 不在 / state あり no-change / state あり check 成功 / state あり check 失敗) を 6 ケースで覆う。 fake repo を t.TempDir() に組み、 git init + 初回 commit + checkFn の fake injection で外部依存を切る。 旧 shell 版 stop-hook-review.sh との parity test は本 PR で cli が shell を呼ばなくなった (FLM_FEA_0004 §責務範囲) ことに伴い撤去する (= 検査経路は bucketize + dispatch の in-process Go ネイティブに統一されたため shell 出力との挙動同一性比較は意味を失う)。
//
// stop hook は cwd 変更 (os.Chdir) を伴うため t.Parallel() と互換が取れない (Go 1.24 testing 規約)。 paralleltest lint を局所抑制する (FLM_GEN_0006 §局所抑制が真に避けられない場合のみ、 理由を併記して例外的に許す)。
//
//nolint:paralleltest // cwd 変更を伴うため parallel 不可
func TestDoRun(t *testing.T) {
	t.Run("agent_id を含む stdin は skip して exit 0", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake, check := newFakeAndCheck(t)
		fake.SetStdin(t, strings.NewReader(`{"agent_id":"sub-1"}`))

		// Act
		err := runWithFakeCheck(t.Context(), fake, check.fn)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
		assert.Equal(t, int32(0), check.callCount.Load(), "checkFn は呼ばれないこと")
		_, statErr := os.Stat(filepath.Join(repo, ".claude", ".cache", "stop-hook", "state.tsv"))
		assert.True(t, os.IsNotExist(statErr), "state は作成されないこと")
	})

	t.Run("stop_hook_active=true を含む stdin は skip して exit 0", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeWorking(t, repo, "lib/x.txt", "hello\n")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake, check := newFakeAndCheck(t)
		fake.SetStdin(t, strings.NewReader(`{"stop_hook_active":true}`))

		// Act
		err := runWithFakeCheck(t.Context(), fake, check.fn)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
		assert.Equal(t, int32(0), check.callCount.Load(), "checkFn は呼ばれないこと")
		_, statErr := os.Stat(filepath.Join(repo, ".claude", ".cache", "stop-hook", "state.tsv"))
		assert.True(t, os.IsNotExist(statErr), "state は作成されないこと")
	})

	t.Run("state 不在の初回起動は state を作成して exit 0", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeWorking(t, repo, "lib/x.txt", "hello\n")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake, check := newFakeAndCheck(t)
		fake.SetStdin(t, strings.NewReader(`{"transcript_path":"x"}`))

		// Act
		err := runWithFakeCheck(t.Context(), fake, check.fn)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
		assert.Equal(t, int32(0), check.callCount.Load(), "初回起動で checkFn は呼ばれないこと")
		state := readStateFile(t, repo)
		assert.Contains(t, state, "lib/x.txt", "untracked ファイルが state に登録されていること")
	})

	t.Run("state 存在 + 変更ファイル無しなら check は呼ばれず exit 0", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeWorking(t, repo, "lib/x.txt", "hello\n")
		seedStateForTrackedFiles(t, repo)
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake, check := newFakeAndCheck(t)
		check.exitCode = exitFailWithMarker
		check.output = "FAIL_MARKER"
		fake.SetStdin(t, strings.NewReader(`{}`))

		// Act
		err := runWithFakeCheck(t.Context(), fake, check.fn)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
		assert.Equal(t, int32(0), check.callCount.Load(), "checkFn は呼ばれないこと")
	})

	t.Run("state 存在 + 変更ファイル有り + check 成功なら state を更新して exit 0", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeWorking(t, repo, "lib/x.txt", "hello\n")
		seedStateForTrackedFiles(t, repo)
		writeWorking(t, repo, "lib/x.txt", "changed\n")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake, check := newFakeAndCheck(t)
		fake.SetStdin(t, strings.NewReader(`{}`))

		// Act
		err := runWithFakeCheck(t.Context(), fake, check.fn)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
		assert.Equal(t, int32(1), check.callCount.Load(), "checkFn が 1 回呼ばれていること")
		assert.Contains(t, check.lastFiles, "lib/x.txt", "変更 file が checkFn に渡されていること")
		newHash := gitHash(t, repo, "lib/x.txt")
		state := readStateFile(t, repo)
		assert.Contains(t, state, "lib/x.txt\t"+newHash, "state は変更後 hash で更新されていること")
	})

	t.Run("state 存在 + 変更ファイル有り + check 失敗なら block JSON を stdout に出して exit 0", func(t *testing.T) {
		// Arrange
		repo := setupRepo(t)
		writeWorking(t, repo, "lib/x.txt", "hello\n")
		seedStateForTrackedFiles(t, repo)
		writeWorking(t, repo, "lib/x.txt", "changed\n")
		t.Setenv("CLAUDE_PROJECT_DIR", repo)
		fake, check := newFakeAndCheck(t)
		check.exitCode = exitFailWithMarker
		check.output = "FAIL_MARKER args=lib/x.txt\n"
		fake.SetStdin(t, strings.NewReader(`{}`))

		// Act
		err := runWithFakeCheck(t.Context(), fake, check.fn)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, expectedBlockJSON(t, "FAIL_MARKER args=lib/x.txt\n"), "")
	})
}

// expectedBlockJSON は emitBlockReason が emit する block decision JSON を test 側で再現する。 expected を Marshal して actual と完全一致比較する形 (FLM_APP_0009 §assertion 規約: actual に Unmarshal しない / expected を Marshal する) を取るため、 production と同じ encoder 設定 (SetEscapeHTML(false) / SetIndent / Encode が末尾改行を付与) で組む。
func expectedBlockJSON(tb testing.TB, staticOutput string) string {
	tb.Helper()
	reason := "ターン終端の静的検査で違反を検出した。 以下を全て修正すること。\n\n" + staticOutput
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

const (
	exitFailWithMarker = 17
)

// fakeCheck は test 内で checkFn を fake する観測 channel + 返り値固定 helper (FLM_APP_0009 §mock を採用しない / fake を採用)。 callCount は in-process でも race detector 安全に観測するため atomic.Int32 を使う。
type fakeCheck struct {
	output    string
	lastFiles []string
	callCount atomic.Int32
	exitCode  int
}

func (f *fakeCheck) fn(_ context.Context, _ string, files []string) (output string, exitCode int, err error) { //nolint:nonamedreturns // gocritic unnamedResult: checkFn の signature に揃えるため named return で 3 つの意味を明示。
	f.callCount.Add(1)
	f.lastFiles = append([]string{}, files...)
	return f.output, f.exitCode, nil
}

func newFakeAndCheck(tb testing.TB) (*clix.FakeIO, *fakeCheck) {
	tb.Helper()
	return clix.NewFakeIO(tb, []string{"stop"}), &fakeCheck{output: "", lastFiles: nil, callCount: atomic.Int32{}, exitCode: 0}
}

// runWithFakeCheck は cobra root を組み、 leaf の RunE 内で doRun に fake checkFn を inject して invoke する。 FakeIO は clix.IO interface 実装で RunInput に直接渡せない (= 内部で cobra wrapper が IO → RunInput 変換する) ため、 root 経由で間接的に呼ぶ。
func runWithFakeCheck(ctx context.Context, io *clix.FakeIO, runCheck checkFn) error {
	r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
	r.AddCommand(clix.NewLeaf("stop", "stop hook (test)", func(innerCtx context.Context, in clix.RunInput) error { //nolint:contextcheck // closure 内で受け取る innerCtx を doRun に渡す経路で外側 ctx は cobra root の Run へ閉じる。
		return doRun(innerCtx, in, runCheck)
	}))
	return r.Run(ctx, io) //nolint:wrapcheck // test helper: 起動失敗時の error は test caller の require.Error / require.NoError で扱うため、 ここでは raw error を返す。
}

// setupRepo は fake repo を t.TempDir() に作り、 git init + 初回 commit を打つ。 stop hook は git ls-files / git diff / git hash-object を呼ぶため、 commit の有る repo が前提。
func setupRepo(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	require.NoError(tb, os.MkdirAll(filepath.Join(root, "lib"), 0o750))
	mustGit(tb, root, "init", "-q", "-b", "main")
	mustGit(tb, root, "config", "user.email", "test@example.com")
	mustGit(tb, root, "config", "user.name", "test")
	// stop hook 自身が `.claude/.cache/stop-hook/` 配下に state.tsv / .lock を作る。 これらが untracked として ls-files に拾われると baseline 比較の対象外ファイルが「changed」 扱いになり、 fixture が意図しない check 起動を招く (本物の repo では root .gitignore で同パスを除外しているのと同じ扱いを fake repo にも適用する)。
	require.NoError(tb, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".claude/.cache/\n"), 0o600))
	require.NoError(tb, os.WriteFile(filepath.Join(root, "README.md"), []byte("seed\n"), 0o600))
	mustGit(tb, root, "add", ".gitignore", "README.md")
	mustGit(tb, root, "commit", "-q", "-m", "init")
	return root
}

// writeWorking は test fixture: working tree 配下の任意 path に file を書き出す。 rel が現状 "lib/x.txt" 固定なのは fake repo の seed 構造に揃えただけで、 将来の test 追加で別 path を扱う前提で param を持つ (unparam 局所抑制)。
//
//nolint:unparam // 上記コメント参照。
func writeWorking(tb testing.TB, repo, rel, body string) {
	tb.Helper()
	full := filepath.Join(repo, rel)
	require.NoError(tb, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(tb, os.WriteFile(full, []byte(body), 0o600))
}

// seedStateForTrackedFiles は現 working tree の状態を baseline として state.tsv に書き込む。 「state 既存」 + 「変更ファイル無し」 シナリオの起点を作るための helper。 stop hook を 1 回起動して state を初期化する代わりに、 直接 state ファイルを作る方式 (test の決定性を優先)。
func seedStateForTrackedFiles(tb testing.TB, repo string) {
	tb.Helper()
	stateDir := filepath.Join(repo, ".claude", ".cache", "stop-hook")
	require.NoError(tb, os.MkdirAll(stateDir, 0o750))
	head := mustGitOutput(tb, repo, "rev-parse", "HEAD")
	head = strings.TrimSpace(head)

	modified := splitLines(mustGitOutput(tb, repo, "diff", "--name-only", "--diff-filter=ACMR", "HEAD"))
	untracked := splitLines(mustGitOutput(tb, repo, "ls-files", "--others", "--exclude-standard"))

	var buf bytes.Buffer
	buf.WriteString(head + "\n")
	seen := map[string]struct{}{}
	for _, f := range append(modified, untracked...) {
		if f == "" {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		full := filepath.Join(repo, f)
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			continue
		}
		h := strings.TrimSpace(mustGitOutput(tb, repo, "hash-object", f))
		buf.WriteString(f + "\t" + h + "\n")
	}
	require.NoError(tb, os.WriteFile(filepath.Join(stateDir, "state.tsv"), buf.Bytes(), 0o600))
}

func readStateFile(tb testing.TB, repo string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join(repo, ".claude", ".cache", "stop-hook", "state.tsv"))
	require.NoError(tb, err)
	return string(data)
}

func gitHash(tb testing.TB, repo, rel string) string {
	tb.Helper()
	return strings.TrimSpace(mustGitOutput(tb, repo, "hash-object", rel))
}

func mustGit(tb testing.TB, repo string, args ...string) {
	tb.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // G204: test fixture: 各 args は test 内 caller 内部値で外部入力ではない。
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	require.NoErrorf(tb, err, "git %s: %s", strings.Join(args, " "), string(out))
}

func mustGitOutput(tb testing.TB, repo string, args ...string) string {
	tb.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // G204: test fixture: 各 args は test 内 caller 内部値で外部入力ではない。
	cmd.Dir = repo
	out, err := cmd.Output()
	require.NoErrorf(tb, err, "git %s", strings.Join(args, " "))
	return string(out)
}

func splitLines(s string) []string {
	out := []string{}
	for l := range strings.SplitSeq(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
