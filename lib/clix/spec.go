package clix

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/wakuwaku3/flame/lib/ex"
)

// newSpecCommand は CLI 公開 surface を JSON で emit する隠し subcommand を返す。 release ワークフロー (FLM_FEA_0002) が版 bump 判定の入力として消費する。 利用者向けの公開 API ではないため Hidden = true、 名前は `__` prefix を付け buildSpec の filter から除外される。 本 endpoint は wrapper が自動で登録するシステムコマンドであり、 root / subcommand 実装 package は意識しない。
func newSpecCommand(root *cobra.Command) *cobra.Command {
	cmd := newCobraCommand()
	cmd.Use = "__spec"
	cmd.Short = "Emit CLI surface JSON (internal release tooling)"
	cmd.Hidden = true
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		s := buildSpec(root)
		return encodeSpec(c.OutOrStdout(), &s)
	}
	return cmd
}

// spec の JSON 表現は release ワークフローが前回 release と比較して MAJOR / MINOR / PATCH を決定する入力 (FLM_FEA_0002)。
type spec struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Subcommands []spec    `json:"subcommands"`
	Flags       []flagDef `json:"flags"`
}

// flagDef は名前 / 型 / required 属性が一致するかどうかで bump 判定の入力になる (FLM_FEA_0002)。
type flagDef struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Required  bool   `json:"required"`
}

func buildSpec(cmd *cobra.Command) spec {
	return buildSpecNode(cmd, cmd.Name())
}

func buildSpecNode(cmd *cobra.Command, path string) spec {
	return spec{
		Name:        cmd.Name(),
		Path:        path,
		Subcommands: collectSubcommands(cmd, path),
		Flags:       collectFlags(cmd),
	}
}

func collectSubcommands(cmd *cobra.Command, path string) []spec {
	children := cmd.Commands()
	subs := make([]spec, 0, len(children))
	for _, child := range children {
		if !child.IsAvailableCommand() {
			continue
		}
		if isInternalName(child.Name()) {
			continue
		}
		subs = append(subs, buildSpecNode(child, path+" "+child.Name()))
	}
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].Name < subs[j].Name
	})
	return subs
}

func collectFlags(cmd *cobra.Command) []flagDef {
	flags := make([]flagDef, 0)
	required := requiredFlagSet(cmd)
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flags = append(flags, flagDef{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Required:  required[f.Name],
		})
	})
	sort.Slice(flags, func(i, j int) bool {
		return flags[i].Name < flags[j].Name
	})
	return flags
}

func requiredFlagSet(cmd *cobra.Command) map[string]bool {
	required := map[string]bool{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		annotations := f.Annotations[cobra.BashCompOneRequiredFlag]
		for _, v := range annotations {
			if v == "true" {
				required[f.Name] = true
			}
		}
	})
	return required
}

// `__` prefix の subcommand は CLI 公開 surface とみなさず spec から除外する (release 機構の内部実装が surface を汚染しないようにするため)。
func isInternalName(name string) bool {
	return strings.HasPrefix(name, "__")
}

func encodeSpec(w io.Writer, s *spec) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return ex.Wrap(enc.Encode(s))
}
