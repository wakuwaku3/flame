// Package clix は cobra への依存を集約する wrapper (FLM_APP_0008)。 公開 API は constructor / functional options のみで、 struct 型 / option 型は全て package private (FLM_APP_0007 §公開 struct の最小化)。
package clix

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/wakuwaku3/flame/lib/ex"
)

type rootConfig struct {
	use     string
	short   string
	version string
}

type rootOption func(*rootConfig)

func WithRootShort(short string) rootOption {
	return func(c *rootConfig) { c.short = short }
}

func NewRootConfig(use, version string, opts ...rootOption) *rootConfig {
	c := &rootConfig{
		use:     use,
		version: version,
		short:   "",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type root struct {
	cmd *cobra.Command
}

func NewRoot(cfg *rootConfig) *root {
	cmd := buildRootCobra(cfg)
	cmd.SetVersionTemplate(cfg.use + " {{.Version}}\n")
	cmd.AddCommand(newSpecCommand(cmd))
	return &root{cmd: cmd}
}

// Subcommand は subcommand を表す sealed interface。 cobra method は package private なので、 当該 interface を満たす値は clix package 内部の `*command` のみ生成できる。 外部 subcommand package はこの interface を返す関数を export する形で「clix の command 値を返す」 規約 (FLM_APP_0008 §subcommand package の階層) を満たす。
type Subcommand interface {
	cobra() *cobra.Command
}

func (r *root) AddCommand(c Subcommand) {
	r.cmd.AddCommand(c.cobra())
}

// cio (clix IO) は package "io" との shadow を避けるための引数名 (gocritic importShadow)。 IO interface は clix が定義する CLI IO 抽象で、 stdlib io.Writer 系列とは別概念。
func (r *root) Run(ctx context.Context, cio IO) error {
	r.cmd.SetArgs(cio.args())
	r.cmd.SetIn(cio.stdin())
	r.cmd.SetOut(cio.stdout())
	r.cmd.SetErr(cio.stderr())
	r.cmd.SetContext(ctx)
	return ex.Wrap(r.cmd.Execute())
}

// RunInput は subcommand の runE が受け取る実行時 input。 第二引数を struct ではなく interface にすることで、 flag 等の追加時に runE の signature を破壊せずに後方互換に拡張できる (FLM_APP_0008)。 Stdin / Stdout / Stderr は service-level test (FLM_APP_0009) のため fake 化された stream を leaf に渡す経路で、 production では os.Stdin / os.Stdout / os.Stderr、 test では FakeIO 内の bytes.Reader / bytes.Buffer に解決される。
type RunInput interface {
	Args() []string
	Stdin() io.Reader
	Stdout() io.Writer
	Stderr() io.Writer
}

type runInput struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	args   []string
}

var _ RunInput = (*runInput)(nil)

func (r *runInput) Args() []string    { return r.args }
func (r *runInput) Stdin() io.Reader  { return r.stdin }
func (r *runInput) Stdout() io.Writer { return r.stdout }
func (r *runInput) Stderr() io.Writer { return r.stderr }

type commandConfig struct {
	runE  func(ctx context.Context, in RunInput) error
	use   string
	short string
}

type commandOption func(*commandConfig)

func WithCommandShort(short string) commandOption {
	return func(c *commandConfig) { c.short = short }
}

// WithCommandRunE を設定しない command は実行不能な中間 command として扱われる (cobra の RunE 未設定 command の挙動)。
func WithCommandRunE(runE func(ctx context.Context, in RunInput) error) commandOption {
	return func(c *commandConfig) { c.runE = runE }
}

func NewCommandConfig(use string, opts ...commandOption) *commandConfig {
	c := &commandConfig{
		runE:  nil,
		use:   use,
		short: "",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type command struct {
	cmd *cobra.Command
}

func (c *command) cobra() *cobra.Command { return c.cmd }

func NewCommand(cfg *commandConfig) *command {
	return &command{cmd: buildSubcommandCobra(cfg)}
}

func (c *command) AddSubcommand(child Subcommand) {
	c.cmd.AddCommand(child.cobra())
}

// NewLeaf は実行可能な leaf subcommand を Subcommand 値として返す syntactic sugar。 各 leaf subcommand package が `New() clix.Subcommand` を 1 行で実装するための helper ([FLM_APP_0008](../../../docs/adr/application/FLM_APP_0008__cli.md) §subcommand package の階層)。
func NewLeaf(use, short string, runE func(ctx context.Context, in RunInput) error) Subcommand {
	return NewCommand(NewCommandConfig(use,
		WithCommandShort(short),
		WithCommandRunE(runE),
	))
}

// newCobraCommand は cobra.Command を全 field 明示初期化で生成する唯一の中央 constructor (exhaustruct / FLM_APP_0007 §公開 struct の最小化)。
func newCobraCommand() *cobra.Command {
	return &cobra.Command{
		Use:                    "",
		Aliases:                nil,
		SuggestFor:             nil,
		Short:                  "",
		GroupID:                "",
		Long:                   "",
		Example:                "",
		ValidArgs:              nil,
		ValidArgsFunction:      nil,
		Args:                   nil,
		ArgAliases:             nil,
		BashCompletionFunction: "",
		Deprecated:             "",
		Annotations:            nil,
		Version:                "",
		PersistentPreRun:       nil,
		PersistentPreRunE:      nil,
		PreRun:                 nil,
		PreRunE:                nil,
		Run:                    nil,
		RunE:                   nil,
		PostRun:                nil,
		PostRunE:               nil,
		PersistentPostRun:      nil,
		PersistentPostRunE:     nil,
		FParseErrWhitelist:     cobra.FParseErrWhitelist{UnknownFlags: false},
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd:         false,
			DisableNoDescFlag:         false,
			DisableDescriptions:       false,
			HiddenDefaultCmd:          false,
			DefaultShellCompDirective: nil,
		},
		TraverseChildren:           false,
		Hidden:                     false,
		SilenceErrors:              false,
		SilenceUsage:               false,
		DisableFlagParsing:         false,
		DisableAutoGenTag:          false,
		DisableFlagsInUseLine:      false,
		DisableSuggestions:         false,
		SuggestionsMinimumDistance: 0,
	}
}

func buildRootCobra(cfg *rootConfig) *cobra.Command {
	cmd := newCobraCommand()
	cmd.Use = cfg.use
	cmd.Short = cfg.short
	cmd.Version = cfg.version
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

func buildSubcommandCobra(cfg *commandConfig) *cobra.Command {
	cmd := newCobraCommand()
	cmd.Use = cfg.use
	cmd.Short = cfg.short
	if cfg.runE != nil {
		runE := cfg.runE
		// cobra の SetIn / SetOut / SetErr は親 command (= clix root) で IO 由来の stream を注入済 (FLM_APP_0009 §service-level test の writer 注入経路)。 InOrStdin / OutOrStdout / ErrOrStderr は親から伝搬した stream を返すため、 production では os.Stdin / os.Stdout / os.Stderr、 test では FakeIO の bytes.Reader / bytes.Buffer が leaf に渡る。
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return runE(c.Context(), &runInput{
				args:   args,
				stdin:  c.InOrStdin(),
				stdout: c.OutOrStdout(),
				stderr: c.ErrOrStderr(),
			})
		}
	}
	return cmd
}
