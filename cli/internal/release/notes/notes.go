// Package notes は release notes 生成 (FLM_FEA_0002 §リリースノート) の共通ロジック。 tool / library 系統で共有される PR 列挙 (compare API + commit message 経由 PR 番号抽出 + label filter) と Markdown 構築を持ち、 install スニペットだけ系統で異なるため caller が文字列で渡す。
package notes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/wakuwaku3/flame/cli/internal/release/ghapi"
	"github.com/wakuwaku3/flame/lib/ex"
)

// HasModuleChangesSincePriorTag は前回 release tag → 今回 commit で当該 module ディレクトリ配下に file change が 1 件でもあれば true を返す (FLM_FEA_0002 §リリース起動契機)。 caller が release を skip する判断に使う。 label PR 件数 (`module/<name>`) で判定する案は fork PR / label 付与前 merge / merge-commit などで誤検知 (実変更があるのに 0 件判定) しやすいため不採用、 GitHub compare API の `.files[].filename` を当該 module dir prefix で絞り込む path ベース判定を採用する。 compare API 失敗 / response 解釈失敗 / compare REST truncation 兆候 (commits が total に届いていない / files が 300 件上限張り付き) を検出した場合は安全側 (= release を進める) で true を返し、 warn にメッセージを残す (shell 版 `module_has_changes_since_prior_tag` と挙動を揃える)。
func HasModuleChangesSincePriorTag(ctx context.Context, warn io.Writer, gh ghapi.Client, repo, priorTag, headSHA, modulePath string) bool {
	if priorTag == "" || headSHA == "" || modulePath == "" {
		return true
	}
	compareJSON, err := gh.API(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", repo, priorTag, headSHA))
	if err != nil {
		fmt.Fprintf(warn, "warn: failed to fetch compare %s...%s; assuming module changes are present\n", priorTag, headSHA)
		return true
	}
	var compare struct {
		Commits []struct {
			SHA string `json:"sha"`
		} `json:"commits"`
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
		TotalCommits int `json:"total_commits"`
	}
	if jsonErr := json.Unmarshal(compareJSON, &compare); jsonErr != nil {
		fmt.Fprintf(warn, "warn: failed to parse compare response (%v); assuming module changes are present\n", jsonErr)
		return true
	}
	if len(compare.Commits) < compare.TotalCommits || len(compare.Files) >= filesTruncationThreshold {
		fmt.Fprintf(warn, "warn: compare REST may be truncated: total_commits=%d returned_commits=%d files=%d; assuming module changes are present\n", compare.TotalCommits, len(compare.Commits), len(compare.Files))
		return true
	}
	prefix := modulePath + "/"
	for _, f := range compare.Files {
		if strings.HasPrefix(f.Filename, prefix) {
			return true
		}
	}
	return false
}

// Compose は release notes Markdown を out に書き出す。 priorTag が空文字なら初版扱いで Changes セクションに `_Initial release._` を出す。 install snippet は系統で異なるため caller が組み立てた文字列を渡す。 in は ComposeInput を pointer で受ける (gocritic hugeParam: 112B 超は pointer 渡し)。
func Compose(ctx context.Context, out, warn io.Writer, in *ComposeInput) error {
	if in.ModuleName == "" {
		return ex.New("module name must be non-empty")
	}
	if in.Heading == "" {
		return ex.New("heading must be non-empty")
	}
	if in.CommitSHA == "" {
		return ex.New("commit sha must be non-empty")
	}

	var prs []pullRequest
	if in.PriorTag != "" {
		var err error
		prs, err = collectPRs(ctx, warn, in.GHAPI, in.Repo, in.PriorTag, in.CommitSHA, "module/"+in.ModuleName)
		if err != nil {
			return ex.Wrap(err)
		}
	}

	fmt.Fprintf(out, "## %s\n", in.Heading)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Auto-generated release for commit `%s`.\n", in.CommitSHA)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "### Changes")
	fmt.Fprintln(out)
	if len(prs) == 0 {
		if in.PriorTag == "" {
			fmt.Fprintln(out, "_Initial release._")
		} else {
			fmt.Fprintf(out, "_No PRs labeled `module/%s` since %s._\n", in.ModuleName, in.PriorTag)
		}
	} else {
		for _, pr := range prs {
			fmt.Fprintf(out, "- #%d %s (@%s) — %s\n", pr.Number, pr.Title, pr.Author, pr.URL)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "### Install")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "```sh")
	fmt.Fprintln(out, in.InstallSnippet)
	fmt.Fprintln(out, "```")
	return nil
}

// ComposeInput は Compose の入力。 InstallSnippet は系統別 (`curl ...` / `go get ...`) のため caller が事前に整形して渡す。
type ComposeInput struct {
	GHAPI          ghapi.Client
	Repo           string
	ModuleName     string
	Heading        string
	CommitSHA      string
	PriorTag       string
	InstallSnippet string
}

type pullRequest struct {
	Title  string
	URL    string
	Author string
	Number int
}

// prNumberPattern は squash-merge default で commit subject 末尾に GitHub が付与する `(#<number>)` から PR 番号を抽出する (FLM_FEA_0002 §リリースノート: 番号抽出経路は immediate consistent な `/pulls/{n}` 解決のために必要)。
var prNumberPattern = regexp.MustCompile(`\(#(\d+)\)\s*$`)

// collectPRs は compare API → 各 commit subject の `(#<n>)` 抽出 → `/pulls/{n}` で PR detail 取得 → label で client 側絞込みの 4 段で PR 列挙を行う (FLM_FEA_0002 §リリースノート)。 compare 失敗時は warn に記録して空 list を返し release 自体は通す経路は compare error を Wrap せず caller に戻すと release が止まるため、 ここで吸収して `(nil, nil)` を返す (nilerr lint は本関数の意図的挙動として suppress)。
func collectPRs(ctx context.Context, warn io.Writer, gh ghapi.Client, repo, priorTag, headSHA, label string) ([]pullRequest, error) {
	if priorTag == "" || headSHA == "" || label == "" {
		return nil, ex.New("priorTag / headSHA / label must be non-empty")
	}
	compareJSON, err := gh.API(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", repo, priorTag, headSHA))
	if err != nil {
		fmt.Fprintf(warn, "warn: failed to fetch compare %s...%s; release notes Changes section will be empty\n", priorTag, headSHA)
		return nil, nil //nolint:nilerr // shell 版 `compose_release_notes` が compare 失敗を warn ログ + 空 PR list で握り潰すのと挙動を揃えるため意図的に nil error を返す。
	}
	var compare struct {
		Commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"commits"`
		TotalCommits int `json:"total_commits"`
	}
	if err := json.Unmarshal(compareJSON, &compare); err != nil {
		return nil, ex.Wrapf(err, "decode compare response")
	}
	if len(compare.Commits) < compare.TotalCommits {
		fmt.Fprintf(warn, "warn: compare REST truncated: total=%d returned=%d; release notes may omit older PRs\n", compare.TotalCommits, len(compare.Commits))
	}

	skipCount := 0
	seen := make(map[int]struct{})
	var prs []pullRequest
	const subjectAndBody = 2
	for _, c := range compare.Commits {
		subject := strings.SplitN(c.Commit.Message, "\n", subjectAndBody)[0]
		match := prNumberPattern.FindStringSubmatch(subject)
		if match == nil {
			fmt.Fprintf(warn, "warn: cannot extract PR number from commit %s; skipping (subject: '%s')\n", shortSHA(c.SHA), preview(subject, prSubjectPreviewLimit))
			skipCount++
			continue
		}
		var prNum int
		if _, err := fmt.Sscanf(match[1], "%d", &prNum); err != nil {
			skipCount++
			continue
		}
		detailJSON, err := gh.API(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repo, prNum))
		if err != nil {
			fmt.Fprintf(warn, "warn: failed to fetch PR detail for #%d (commit %s); skipping\n", prNum, shortSHA(c.SHA))
			skipCount++
			continue
		}
		var detail struct {
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
			User    struct {
				Login string `json:"login"`
			} `json:"user"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
			Number int `json:"number"`
		}
		if err := json.Unmarshal(detailJSON, &detail); err != nil {
			skipCount++
			continue
		}
		if !hasLabel(detail.Labels, label) {
			continue
		}
		if _, dup := seen[detail.Number]; dup {
			continue
		}
		seen[detail.Number] = struct{}{}
		prs = append(prs, pullRequest{
			Number: detail.Number,
			Title:  detail.Title,
			URL:    detail.HTMLURL,
			Author: detail.User.Login,
		})
	}
	if skipCount > 0 {
		fmt.Fprintf(warn, "warn: skipped %d commit(s) due to PR resolution failure; release notes Changes section may be incomplete\n", skipCount)
	}
	sort.SliceStable(prs, func(i, j int) bool { return prs[i].Number < prs[j].Number })
	return prs, nil
}

const (
	prSubjectPreviewLimit = 80
	shortSHALen           = 7
	// filesTruncationThreshold は GitHub compare REST が files を truncate する境界 (300 件)。 ここに張り付いている場合 truncation の可能性があるため、 path 判定を信頼せず安全側で true を返す。
	filesTruncationThreshold = 300
)

func hasLabel(labels []struct {
	Name string `json:"name"`
}, target string,
) bool {
	for _, l := range labels {
		if l.Name == target {
			return true
		}
	}
	return false
}

func shortSHA(sha string) string {
	if len(sha) <= shortSHALen {
		return sha
	}
	return sha[:shortSHALen]
}

func preview(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}
