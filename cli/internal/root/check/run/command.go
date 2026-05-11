// Package run は `flame check run <bucket-key>` subcommand。 CI matrix の 1 entry (= bucketize key と当該 bucket のファイル群) を受けて単一 checker を in-process dispatch する。 ファイル一覧は FILES_JSON env (JSON 配列) から読む — matrix output を `${{ toJson(matrix.files) }}` で受け渡す GitHub Actions の標準慣習に揃え、 inline shell の制限 (FLM_ENG_0003) 下で 1 行 invocation に収めるため。
package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wakuwaku3/flame/cli/internal/check/dispatch"
	"github.com/wakuwaku3/flame/lib/clix"
	"github.com/wakuwaku3/flame/lib/ex"
)

const (
	exitCodeUsage   = 2
	exitCodeFailure = 1
)

// envFilesJSON は matrix.files を JSON 配列で受け渡す step 入力 (workflow 側で `FILES_JSON: ${{ toJson(matrix.files) }}` を渡す前提)。
const envFilesJSON = "FILES_JSON"

func New() clix.Subcommand {
	return clix.NewLeaf("run", "matrix entry を 1 checker に dispatch する (FILES_JSON env でファイル一覧を受ける)", Run)
}

func Run(ctx context.Context, in clix.RunInput) error {
	args := in.Args()
	if len(args) != 1 {
		fmt.Fprintln(in.Stderr(), "usage: flame check run <bucket-key> (FILES_JSON env required)")
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	key := args[0]
	runner, ok := dispatch.Registry[key]
	if !ok {
		fmt.Fprintf(in.Stderr(), "error: unknown checker key %q\n", key)
		return ex.Wrap(clix.NewExitError(exitCodeUsage))
	}
	files, err := readFilesEnv()
	if err != nil {
		fmt.Fprintf(in.Stderr(), "error: %v\n", err)
		return ex.Wrap(clix.NewExitError(exitCodeFailure))
	}
	if len(files) == 0 {
		fmt.Fprintf(in.Stderr(), "no files to check for %s\n", key)
		return nil
	}
	for _, f := range files {
		if f != "" && f[0] == '-' {
			fmt.Fprintf(in.Stderr(), "error: invalid file name starting with '-': %s\n", f)
			return ex.Wrap(clix.NewExitError(exitCodeFailure))
		}
	}
	scoped := &scopedInput{base: in, args: files}
	return ex.Wrap(runner(ctx, scoped))
}

func readFilesEnv() ([]string, error) {
	raw, ok := os.LookupEnv(envFilesJSON)
	if !ok || raw == "" {
		return nil, ex.Errorf("%s env must be set to a JSON array of file paths", envFilesJSON)
	}
	var files []string
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return nil, ex.Wrapf(err, "%s is not a JSON string array", envFilesJSON)
	}
	return files, nil
}

// scopedInput は親 RunInput の stdin / stdout / stderr を継承しつつ Args だけを当該 checker 向けの files に置き換えた wrapper。
type scopedInput struct {
	base clix.RunInput
	args []string
}

var _ clix.RunInput = (*scopedInput)(nil)

func (s *scopedInput) Args() []string                { return s.args }
func (s *scopedInput) Stdin() io.Reader              { return s.base.Stdin() }
func (s *scopedInput) Stdout() io.Writer             { return s.base.Stdout() }
func (s *scopedInput) Stderr() io.Writer             { return s.base.Stderr() }
func (s *scopedInput) BoolFlag(name string) bool     { return s.base.BoolFlag(name) }
func (s *scopedInput) StringFlag(name string) string { return s.base.StringFlag(name) }
