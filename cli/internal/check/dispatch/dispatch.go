// Package dispatch は bucketize 結果を各 checker に並列で振り分けて実行する dispatcher (FLM_FEA_0001 §checker の独立性)。 cli/internal/root/check/* の各 checker package の Run を in-process で呼ぶ (FLM_FEA_0005 §shell が許される例外: cli は cli/scripts/install.sh と .github/workflows/tests/*.sh 以外の shell を呼ばない)。
package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/wakuwaku3/flame/cli/internal/check/bucketize"
	"github.com/wakuwaku3/flame/cli/internal/root/check/adr"
	"github.com/wakuwaku3/flame/cli/internal/root/check/devbox"
	"github.com/wakuwaku3/flame/cli/internal/root/check/document"
	"github.com/wakuwaku3/flame/cli/internal/root/check/flow_document"
	"github.com/wakuwaku3/flame/cli/internal/root/check/github_actions"
	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/build"
	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/lint"
	"github.com/wakuwaku3/flame/cli/internal/root/check/golang/test"
	"github.com/wakuwaku3/flame/cli/internal/root/check/json"
	"github.com/wakuwaku3/flame/cli/internal/root/check/shell"
	"github.com/wakuwaku3/flame/cli/internal/root/check/yaml"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

type CheckerFunc = func(ctx context.Context, in clix.RunInput) error

var Registry = map[string]CheckerFunc{
	bucketize.CheckerDocument:      document.Run,
	bucketize.CheckerADR:           adr.Run,
	bucketize.CheckerShell:         shell.Run,
	bucketize.CheckerJSON:          json.Run,
	bucketize.CheckerYAML:          yaml.Run,
	bucketize.CheckerDevbox:        devbox.Run,
	bucketize.CheckerFlowDocument:  flow_document.Run,
	bucketize.CheckerGitHubActions: github_actions.Run,
	bucketize.CheckerGoLint:        lint.Run,
	bucketize.CheckerGoBuild:       build.Run,
	bucketize.CheckerGoTest:        test.Run,
}

// Dispatch は entries を parallelism 同時実行で並列起動し、 各 checker の stdout / stderr / exit code を集約する。 個別 checker の stdout / stderr は両方とも 1 つの output 文字列に積み、 exit code は max を返す。 entries 中に未知 Checker 名があれば error を返し以降の dispatch を行わない (bucket key と registry の乖離 bug を顕在化させる)。 ctx cancel は子 checker に伝播する。
func Dispatch(ctx context.Context, entries []bucketize.Entry, parallelism int) (output string, exitCode int, err error) { //nolint:nonamedreturns // gocritic unnamedResult: 戻り値 3 つの意味を named return で明示。
	if len(entries) == 0 {
		return "", 0, nil
	}
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}
	for _, e := range entries {
		if _, ok := Registry[e.Checker]; !ok {
			return "", 0, ex.Errorf("unknown checker key %q in dispatch registry", e.Checker)
		}
	}
	results := make([]string, len(entries))
	codes := make([]int, len(entries))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for i, e := range entries {
		wg.Add(1)
		go func(i int, e bucketize.Entry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], codes[i] = runOne(ctx, e)
		}(i, e)
	}
	wg.Wait()
	maxCode := 0
	var combined strings.Builder
	for i, r := range results {
		if r != "" {
			combined.WriteString(r)
		}
		if codes[i] > maxCode {
			maxCode = codes[i]
		}
	}
	return combined.String(), maxCode, nil
}

// runOne は 1 checker を in-process で起動し、 stdout + stderr を 1 つの string にまとめて exit code とともに返す。 clix.NewExitError 由来 (= 個別 checker が `return clix.NewExitError(code)` した経路) はその code を採用し、 それ以外の error 経路は generic に code 1 として扱う (rc != 0 を一律 fail 扱いする)。
func runOne(ctx context.Context, e bucketize.Entry) (output string, exitCode int) { //nolint:nonamedreturns // gocritic unnamedResult: 戻り値 2 つの意味を named return で明示。
	runner := Registry[e.Checker]
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	in := &checkerIO{args: e.Targets, stdin: bytes.NewReader(nil), stdout: stdout, stderr: stderr}
	err := runner(ctx, in)
	out := stdout.String() + stderr.String()
	if err == nil {
		return out, 0
	}
	if code, ok := clix.ExitCodeOf(err); ok {
		return out, code
	}
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + fmt.Sprintf("%v\n", err), 1
}

// checkerIO は dispatch 内部だけで使う clix.RunInput 実装。 lib 側で RunInput interface の method が exported されているため caller 側で直接 implement でき、 in-process invocation が実現できる (subprocess + os.Executable に依存せず Go process 内に閉じる)。
type checkerIO struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	args   []string
}

var _ clix.RunInput = (*checkerIO)(nil)

func (c *checkerIO) Args() []string           { return c.args }
func (c *checkerIO) Stdin() io.Reader         { return c.stdin }
func (c *checkerIO) Stdout() io.Writer        { return c.stdout }
func (c *checkerIO) Stderr() io.Writer        { return c.stderr }
func (*checkerIO) BoolFlag(_ string) bool     { return false }
func (*checkerIO) StringFlag(_ string) string { return "" }
