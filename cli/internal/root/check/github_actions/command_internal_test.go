// service-level test (= command_test.go) では actionlint 外部 process が `uses:` job への `steps:` 混入を独自 stderr/stdout に書き、 出力全体比較が actionlint version 揺らぎを受ける (FLM_APP_0009 §service-level test の「外部 process 出力に依存する場合」 緩和)。 一方、 forbidden-key 検出経路 (FLM_ENG_0003 §トリガー層 で禁止される `steps` / `run` / `runs-on` / `needs`) は独自 FAIL 行 (`trg__ job must not contain ...`) を出力する独立 branch で、 当該行の存在を確認する観点は service-level test では actionlint との出力衝突で網羅困難。 当 ファイル (white-box internal test) で `checkWorkflow` を直接呼んで FAIL 行を返り値で検証する形に分担する (FLM_APP_0009 §unit test の責務)。
package github_actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckWorkflow_TrgForbiddenJobKey(t *testing.T) {
	t.Parallel()

	// Arrange
	const trgBodyWithSteps = `name: pr-check
on:
  push:
    branches: [main]
jobs:
  check:
    name: trg__push__main / check
    uses: ./wf.yaml
    steps:
      - run: echo ok
`
	dir := t.TempDir()
	path := filepath.Join(dir, "trg__push__main.yaml")
	require.NoError(t, os.WriteFile(path, []byte(trgBodyWithSteps), 0o600))
	expected := []string{
		path + `: trg__ job must not contain ["steps"] (FLM_ENG_0003)`,
		path + `: trg__ job may only contain ["name","permissions","secrets","uses","with"]; found extras ["steps"] (FLM_ENG_0003)`,
	}

	// Act
	violations := checkWorkflow(path)

	// Assert
	assert.Equal(t, expected, violations)
}
