package github_actions_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/github_actions"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

// service-level test は actionlint 外部バイナリに依存する。 PATH に存在しない環境では skip し、 devbox 配下 / CI など PATH に actionlint がある環境でのみ検査する (FLM_APP_0009 §service-level test)。 第一引数は `*testing.T` ではなく `testing.TB` interface を取る (FLM_APP_0009 §test helper signature)。
func requireActionlint(tb testing.TB) {
	tb.Helper()
	if _, err := exec.LookPath("actionlint"); err != nil {
		tb.Skipf("actionlint not found in PATH: %v", err)
	}
}

// writeWorkflow は workflow YAML 本体を書き出すと同時に、 FLM_ENG_0003 §test が要求する対応 test script (`<dir>/tests/<stem>.sh`) も exit 0 stub として配置する。 stub を書かないと test runner 統合 (FLM_ENG_0003 §test) の「対応 test script 不在」 検出が全 service-level test を破壊するため、 既存ケースの focus (= 静的検査の出力検査) を保つには happy-path stub の同時生成が必要。 lint 違反と test 不在を独立に検証するケースは別 helper (writeWorkflowOnly) を使う。 第一引数は `testing.TB` を取る (FLM_APP_0009 §test helper signature)。
func writeWorkflow(tb testing.TB, path, body string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(path, []byte(body), 0o600))

	if !strings.HasSuffix(path, ".yaml") {
		return
	}
	dir := filepath.Dir(path)
	stem := strings.TrimSuffix(filepath.Base(path), ".yaml")
	testDir := filepath.Join(dir, "tests")
	require.NoError(tb, os.MkdirAll(testDir, 0o750))
	testPath := filepath.Join(testDir, stem+".sh")
	// bash <path> 起動経由のため exec bit は不要。 0o600 に揃えることで gosec G306 (>= 0o700 制限) を回避する。
	require.NoError(tb, os.WriteFile(testPath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o600))
}

// writeWorkflowOnly は test script stub を伴わずに workflow 本体だけを書き出す。 「test script が無いと FAIL を出す」 ケースの service-level test に使う。 第一引数は `testing.TB` を取る (FLM_APP_0009 §test helper signature)。
func writeWorkflowOnly(tb testing.TB, path, body string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(path, []byte(body), 0o600))
}

// writeFailingTestScript は対応 test script を非 0 終了で配置する。 「test script が失敗すると exit 1 を伝搬する」 ケースの service-level test に使う。 第一引数は `testing.TB` を取る (FLM_APP_0009 §test helper signature)。
func writeFailingTestScript(tb testing.TB, workflowPath string) {
	tb.Helper()
	require.True(tb, strings.HasSuffix(workflowPath, ".yaml"))
	dir := filepath.Dir(workflowPath)
	stem := strings.TrimSuffix(filepath.Base(workflowPath), ".yaml")
	testDir := filepath.Join(dir, "tests")
	require.NoError(tb, os.MkdirAll(testDir, 0o750))
	testPath := filepath.Join(testDir, stem+".sh")
	require.NoError(tb, os.WriteFile(testPath, []byte("#!/usr/bin/env bash\necho 'FAIL: synthetic test failure' >&2\nexit 1\n"), 0o600))
}

// 第一引数 `tb testing.TB` で helper signature を統一しつつ (FLM_APP_0009 §test helper signature)、 ctx は caller subtest が `t.Context()` で取得して第 2 引数として渡す (FLM_APP_0007 §context 伝搬 §影響: service-level test は `t.Context()` で root を駆動する)。 両 ADR を同時に満たすため、 helper 内では `context.Background()` を取らず caller 由来 ctx をそのまま伝搬する。
//
// contextcheck は「ctx を持つ関数は transitive 呼出先全てに ctx を thread すべし」 を検査するが、 `github_actions.New()` は cobra command tree を構築するだけの constructor で ctx を受けない。 cobra の RunE lambda 経由で ctx は `r.Run(ctx, fake)` から伝搬される (`buildSubcommandCobra` 参照)。 constructor pattern に対する contextcheck の false positive は ADR / config レベルでは抑制不能なため、 当該 1 行に限り FLM_GEN_0006 §局所抑制が真に避けられない場合のみ に従って明示的に局所抑制する。
//
//nolint:contextcheck // constructor (github_actions.New()) は ctx を受けず、 ctx は r.Run() 経由で cobra の RunE lambda へ伝搬する。
func runCheck(tb testing.TB, ctx context.Context, args []string) (*clix.FakeIO, error) {
	tb.Helper()
	r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
	r.AddCommand(github_actions.New())
	fake := clix.NewFakeIO(tb, append([]string{"github-actions"}, args...))
	return fake, ex.Wrap(r.Run(ctx, fake))
}

func TestRun(t *testing.T) {
	t.Parallel()

	// minimalValidTrg / minimalValidWf は本 Test* 関数内の subtest 群でのみ参照される。 別 Test* との共有は無いため FLM_APP_0009 §test 内 identifier scope に従い関数 scope に置く。
	const minimalValidTrg = `name: pr-check
on:
  pull_request:
    types: [opened, synchronize, reopened]
jobs:
  check:
    name: trg__pull_request__opened_synchronize_reopened / check
    uses: ./.github/workflows/wf__check.yaml
`
	const minimalValidWf = `name: check
on:
  workflow_call:
    inputs:
      base_sha:
        description: base
        required: true
        type: string
  workflow_dispatch:
    inputs:
      base_sha:
        description: base
        required: true
        type: string
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - name: noop
        run: echo ok
`

	t.Run("引数なし起動は usage を stderr に出して exit code 2 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		const expectedStderr = "usage: flame check github-actions <workflow_file>...\n"

		// Act
		fake, err := runCheck(t, t.Context(), nil)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("拡張子が .yaml でないと FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		ymlPath := filepath.Join(dir, "trg__push__main.yml")
		writeWorkflow(t, ymlPath, minimalValidTrg)
		expectedStderr := "FAIL: " + ymlPath + ": GitHub Actions workflow must use the '.yaml' extension; FLM_ENG_0003 standardizes on '.yaml' (found other suffix)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{ymlPath})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("trg__ / wf__ いずれの prefix も持たないと FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "ci.yaml")
		// actionlint pass する最小 valid GA workflow。
		body := `name: ci
on:
  push:
    branches: [main]
jobs:
  hello:
    name: ci / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": filename must start with 'trg__', 'wf__', or 'flame-trg__' (install copy) (FLM_ENG_0003 / FLM_FEA_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("YAML として parse できないと exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		// 当該ケースは actionlint 自身も parse 失敗を訴えて stderr に独自フォーマットで出力する。 出力全体は actionlint version で揺らぐため、 exit code のみを検査する (FLM_APP_0009 §service-level test の「外部 process 出力に依存する場合」 緩和)。
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__main.yaml")
		writeWorkflow(t, path, "key: [unclosed\n")

		// Act
		_, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
	})

	t.Run("全要件を満たす trg__ ファイルなら no-op success", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__pull_request__opened_synchronize_reopened.yaml")
		writeWorkflow(t, path, minimalValidTrg)

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("全要件を満たす wf__ ファイルなら no-op success", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		writeWorkflow(t, path, minimalValidWf)

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("trg__ filename が segment 規約に違反すると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		// `Push` (大文字) を含むため segInner regex に違反する。 universal な job name prefix も同時違反させないため job name 側もファイル stem で揃える。
		path := filepath.Join(dir, "trg__Push__main.yaml")
		body := `name: pr-check
on:
  push:
    branches: [main]
jobs:
  check:
    name: trg__Push__main / check
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": trg__ filename must match '[flame-]trg__<event>__<discriminator>.yaml' (FLM_ENG_0003 / FLM_FEA_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("trg__ event 部が known event でないと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__bogus__main.yaml")
		body := `name: x
on:
  push:
    branches: [main]
jobs:
  check:
    name: trg__bogus__main / check
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": event segment is not a known GitHub Actions event name (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("trg__ top-level に余計な key があると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__main.yaml")
		body := `name: pr-check
permissions:
  contents: read
on:
  push:
    branches: [main]
jobs:
  check:
    name: trg__push__main / check
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + `: trg__ workflow may only contain top-level keys ["jobs","name","on"]; found extras ["permissions"] (FLM_ENG_0003)` + "\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("trg__ on の event 名がファイル名と一致しないと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__main.yaml")
		body := `name: pr-check
on:
  pull_request:
    types: [opened]
jobs:
  check:
    name: trg__push__main / check
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": 'on' must declare exactly the single event 'push' matching the filename (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("trg__ jobs が 2 件あると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__main.yaml")
		body := `name: pr-check
on:
  push:
    branches: [main]
jobs:
  a:
    name: trg__push__main / a
    uses: ./wf.yaml
  b:
    name: trg__push__main / b
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": 'jobs' must contain exactly one entry (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("activity_type_event の discriminator が types を _ で連結したものと一致しないと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__pull_request__opened.yaml")
		body := `name: pr-check
on:
  pull_request:
    types: [opened, synchronize]
jobs:
  check:
    name: trg__pull_request__opened / check
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": discriminator 'opened' must equal types joined by '_' = 'opened_synchronize' (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("branch_filter_event の discriminator 'all' は branches/tags 無指定で OK", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__all.yaml")
		body := `name: pr-check
on:
  push:
jobs:
  check:
    name: trg__push__all / check
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("branch_filter_event の discriminator が branches リストと一致しないと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__main.yaml")
		body := `name: pr-check
on:
  push:
    branches: [develop]
jobs:
  check:
    name: trg__push__main / check
    uses: ./wf.yaml
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": discriminator 'main' must match on.push.branches or on.push.tags as a single-element list (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("wf__ filename が規約と合わないと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		// `Check` (大文字) を含むため segInner regex に違反する。 universal な job name prefix も同時違反させないため job name 側もファイル stem で揃える。
		path := filepath.Join(dir, "wf__Check.yaml")
		body := `name: check
on:
  workflow_call:
    inputs:
      base_sha:
        description: base
        required: true
        type: string
  workflow_dispatch:
    inputs:
      base_sha:
        description: base
        required: true
        type: string
jobs:
  hello:
    name: wf__Check / hello
    runs-on: ubuntu-latest
    steps:
      - name: noop
        run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": wf__ filename must match 'wf__<verb>[__<target>].yaml' (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("wf__ で workflow_dispatch が欠けると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": wf__ workflow must declare 'on.workflow_dispatch' (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("wf__ inputs の同名 input で type が一致しないと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
    inputs:
      base_sha:
        type: string
  workflow_dispatch:
    inputs:
      base_sha:
        type: boolean
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": input 'base_sha' has mismatched 'type' between workflow_call and workflow_dispatch (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("wf__ inputs に片側だけある input があると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
    inputs:
      a:
        type: string
      b:
        type: string
  workflow_dispatch:
    inputs:
      a:
        type: string
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + `: workflow_call.inputs and workflow_dispatch.inputs must declare the same input names; only in workflow_call=["b"], only in workflow_dispatch=[] (FLM_ENG_0003)` + "\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("fetch-depth: 0 が任意 path にあると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": 'fetch-depth: 0' is forbidden at .jobs.hello.steps[0].with.fetch-depth (FLM_ENG_0003 §最小 clone — use ref: refs/pull/<n>/head or a fixed head_sha and fetch base SHA individually)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("inline run block が 3 行を超えると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - name: long inline
        run: |
          echo line1
          echo line2
          echo line3
          echo line4
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": inline 'run:' block at .jobs.hello.steps[0] exceeds limits (lines=4, chars=44; max 3 lines / 300 chars) — extract to a flame CLI subcommand (FLM_ENG_0003 §inline shell の制限 / FLM_FEA_0005)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("job に name: が無いと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + `: every job must declare 'name:' (FLM_ENG_0003 §ジョブの命名); missing on ["hello"]` + "\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("job name: がファイル stem prefix で始まらないと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: bogus / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + `: every job 'name:' must start with 'wf__check / ' (FLM_ENG_0003 §ジョブの命名); offenders [{"key":"hello","name":"bogus / hello"}]` + "\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("英語散文コメントが 15 文字以上含まれると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `# This is a long English prose comment line
name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ":1: English-prose comment detected (FLM_APP_0001 §自然言語); rewrite in Japanese: 'This is a long English prose comment line'\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("path 様の英語コメント (action ref / version) は許容する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		// `actions/checkout@v4.3.1` のような token は path 様で letter-only でないため許容される。
		body := `# actions/checkout@v4.3.1 docs/notes/example.md
name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("yamllint / actionlint directive コメントは許容する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `# yamllint disable-line rule:line-length here
name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("$GITHUB_OUTPUT への書き込みで tee -a が無いと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - id: e
        run: echo "k=v" >> "$GITHUB_OUTPUT"
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + `:11: '>>"$GITHUB_OUTPUT"' without 'tee -a' is forbidden (FLM_ENG_0003 §step 出力の可観測性); pipe through 'tee -a' so the value is visible in the CI log too` + "\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("$GITHUB_OUTPUT への書き込みが tee -a 経由なら許容する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - id: e
        run: echo "k=v" | tee -a "$GITHUB_OUTPUT"
`
		writeWorkflow(t, path, body)

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("複数ファイル同時起動で全件 valid なら success", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		trgPath := filepath.Join(dir, "trg__pull_request__opened_synchronize_reopened.yaml")
		wfPath := filepath.Join(dir, "wf__check.yaml")
		writeWorkflow(t, trgPath, minimalValidTrg)
		writeWorkflow(t, wfPath, minimalValidWf)

		// Act
		fake, err := runCheck(t, t.Context(), []string{trgPath, wfPath})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("対応する test script が無いと FAIL を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__pull_request__opened_synchronize_reopened.yaml")
		writeWorkflowOnly(t, path, minimalValidTrg)
		expectedTestScript := filepath.Join(dir, "tests", "trg__pull_request__opened_synchronize_reopened.sh")
		expectedStderr := "FAIL: " + path + ": missing corresponding test script at " + expectedTestScript + " (FLM_ENG_0003 §test)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("対応する test script が exit 1 で終わると lint pass でも exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		writeWorkflowOnly(t, path, minimalValidWf)
		writeFailingTestScript(t, path)
		const expectedStderr = "FAIL: synthetic test failure\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("wf__ で workflow_call が欠けると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_dispatch:
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedStderr := "FAIL: " + path + ": wf__ workflow must declare 'on.workflow_call' (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("対応 test script の path に directory があると FAIL を出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		writeWorkflowOnly(t, path, minimalValidWf)
		// `<dir>/tests/wf__check.sh` を file ではなく directory として作成し、 runWorkflowTest の info.IsDir() 分岐を起動する。
		testDirPath := filepath.Join(dir, "tests", "wf__check.sh")
		require.NoError(t, os.MkdirAll(testDirPath, 0o750))
		expectedStderr := "FAIL: " + path + ": expected test script at " + testDirPath + " but found a directory (FLM_ENG_0003 §test)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("複数ファイル同時起動で片方のみ違反すると invalid 側の FAIL のみ出して exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		validPath := filepath.Join(dir, "trg__pull_request__opened_synchronize_reopened.yaml")
		writeWorkflow(t, validPath, minimalValidTrg)
		invalidPath := filepath.Join(dir, "trg__push__main.yaml")
		invalidBody := `name: pr-check
on:
  pull_request:
    types: [opened]
jobs:
  check:
    name: trg__push__main / check
    uses: ./wf.yaml
`
		writeWorkflow(t, invalidPath, invalidBody)
		expectedStderr := "FAIL: " + invalidPath + ": 'on' must declare exactly the single event 'push' matching the filename (FLM_ENG_0003)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{validPath, invalidPath})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	const minimalValidFlameTrg = `name: deploy-on-main
on:
  push:
    branches:
      - main
jobs:
  deploy:
    name: trg__push__main / deploy
    uses: wakuwaku3/flame/.github/workflows/wf__deploy.yaml@main
`

	t.Run("flame-trg__ install copy filename を受理し test script を vendor SoT path から resolve する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		repoRoot := t.TempDir()
		workflowDir := filepath.Join(repoRoot, ".github", "workflows")
		require.NoError(t, os.MkdirAll(workflowDir, 0o750))
		workflowPath := filepath.Join(workflowDir, "flame-trg__push__main.yaml")
		require.NoError(t, os.WriteFile(workflowPath, []byte(minimalValidFlameTrg), 0o600))
		vendorTestDir := filepath.Join(repoRoot, "vendor", "flame", ".github", "workflows", "tests")
		require.NoError(t, os.MkdirAll(vendorTestDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(vendorTestDir, "trg__push__main.sh"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o600))

		// Act
		fake, err := runCheck(t, t.Context(), []string{workflowPath})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("flame-trg__ install copy で vendor 側 test script が無いと missing 表示で exit code 1 を伝搬する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		repoRoot := t.TempDir()
		workflowDir := filepath.Join(repoRoot, ".github", "workflows")
		require.NoError(t, os.MkdirAll(workflowDir, 0o750))
		workflowPath := filepath.Join(workflowDir, "flame-trg__push__main.yaml")
		require.NoError(t, os.WriteFile(workflowPath, []byte(minimalValidFlameTrg), 0o600))
		expectedTestPath := filepath.Join(repoRoot, "vendor", "flame", ".github", "workflows", "tests", "trg__push__main.sh")
		expectedStderr := "FAIL: " + workflowPath + ": missing corresponding test script at " + expectedTestPath + " (FLM_ENG_0003 §test)\n"

		// Act
		fake, err := runCheck(t, t.Context(), []string{workflowPath})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("trg__ job に forbidden key (steps:) が含まれると専用 FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__pull_request__opened_synchronize_reopened.yaml")
		body := `name: pr-check
on:
  pull_request:
    types: [opened, synchronize, reopened]
jobs:
  check:
    name: trg__pull_request__opened_synchronize_reopened / check
    uses: ./.github/workflows/wf__check.yaml
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		// actionlint も同 violation を独自 format で出力するため、 lint 観点だけを見る目的で
		// 当該 FAIL 行が stderr に含まれることのみを assertion する (FLM_APP_0009 §service-level test の
		// 「外部 process 出力に依存する場合」 緩和、 line 180 と同じ判断)。
		expectedForbiddenLine := "FAIL: " + path + ": trg__ job must not contain [\"steps\"] (FLM_ENG_0003)"
		expectedExtrasLine := "FAIL: " + path + ": trg__ job may only contain [\"name\",\"permissions\",\"secrets\",\"uses\",\"with\"]; found extras [\"steps\"] (FLM_ENG_0003)"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		stderr := fake.StderrString(t)
		assert.Contains(t, stderr, expectedForbiddenLine)
		assert.Contains(t, stderr, expectedExtrasLine)
	})

	t.Run("trg__ job に uses: が無いと dispatch 規約違反 FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__pull_request__opened_synchronize_reopened.yaml")
		body := `name: pr-check
on:
  pull_request:
    types: [opened, synchronize, reopened]
jobs:
  check:
    name: trg__pull_request__opened_synchronize_reopened / check
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`
		writeWorkflow(t, path, body)
		expectedDispatchLine := "FAIL: " + path + ": trg__ job must dispatch via 'uses:' (FLM_ENG_0003)"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), expectedDispatchLine)
	})

	t.Run("branch_filter_event の discriminator 'all' は branches: ['**'] 単独でも受理する", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__all.yaml")
		body := `name: push-all
on:
  push:
    branches: ['**']
jobs:
  check:
    name: trg__push__all / check
    uses: ./.github/workflows/wf__check.yaml
`
		writeWorkflow(t, path, body)
		// test script は exit 1 で終わる helper を踏襲しつつ、 本ケースでは lint 経路の OK 判定だけを見たいので
		// 本 dir 専用の no-op test script を用意して test 経路を pass させる。 0o600 は gosec G306 を満たす最小権限。
		testDir := filepath.Join(dir, "tests")
		require.NoError(t, os.MkdirAll(testDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(testDir, "trg__push__all.sh"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o600))

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("wf__ の workflow_call.inputs に malformed (mapping ではない) entry があると FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "wf__check.yaml")
		body := `name: check
on:
  workflow_call:
    inputs:
      base_sha:
  workflow_dispatch:
    inputs:
      base_sha:
        description: base
        required: true
        type: string
jobs:
  hello:
    name: wf__check / hello
    runs-on: ubuntu-latest
    steps:
      - name: noop
        run: echo ok
`
		writeWorkflow(t, path, body)
		// actionlint も "type" 不在を独自 format で出力するため stderr 内の本 FAIL 行 substring のみを
		// assert する (FLM_APP_0009 緩和)。
		expectedMalformedLine := "FAIL: " + path + ": input 'base_sha' is malformed (each side must be a mapping with an explicit 'type:' key) (FLM_ENG_0003)"

		// Act
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), expectedMalformedLine)
	})

	t.Run("on: が mapping ではなく shorthand scalar (on: push) だと FAIL を出す", func(t *testing.T) {
		t.Parallel()
		requireActionlint(t)

		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "trg__push__main.yaml")
		body := `name: push-main
on: push
jobs:
  check:
    name: trg__push__main / check
    uses: ./.github/workflows/wf__check.yaml
`
		writeWorkflowOnly(t, path, body)
		// actionlint も "on" must be mapping 等を訴える。 prefix 規約検査までは到達しても、 後続の trg__ event 検査で "on" parse が必要となり、 mapping ではないことを検出する経路が独立に発火する。
		// expected stderr の確実な部分文字列のみ検査するため substring 含有を assert する。
		fake, err := runCheck(t, t.Context(), []string{path})

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		assert.Contains(t, fake.StderrString(t), path+":")
	})
}
