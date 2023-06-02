package clio

import (
	"context"
	"fmt"
	"strings"

	"github.com/gookit/color"
	"github.com/pborman/indent"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
)

type Initializer func(state *State) error

type Application interface {
	ID() Identification
	Run(fn func(ctx context.Context) error) func(cmd *cobra.Command, args []string) error
	SetupCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command
}

type application struct {
	setup SetupConfig `yaml:"-" mapstructure:"-"`
	state State       `yaml:"-" mapstructure:"-"`
}

func New(cfg SetupConfig) (Application, *cobra.Command) {
	a := &application{
		setup: cfg,
	}

	return a, setupRootCmd(a)
}

func nonNil(a ...any) []any {
	var ret []any
	for _, v := range a {
		if v != nil {
			ret = append(ret, v)
		}
	}
	return ret
}

func (a application) ID() Identification {
	return a.setup.ID
}

func (a *application) PostLoad() error {
	a.state.setupBus(a.setup.BusConstructor)

	if err := a.state.setupLogger(a.setup.LoggerConstructor); err != nil {
		return fmt.Errorf("unable to setup logger: %w", err)
	}

	if err := a.state.setupUI(a.setup.UIConstructor); err != nil {
		return fmt.Errorf("unable to setup UI: %w", err)
	}

	for _, init := range a.setup.Initializers {
		if err := init(&a.state); err != nil {
			return err
		}
	}
	return nil
}

// TODO: configs of any doesn't lean into the type system enough. Consider a more specific type.

func (a *application) Setup(cfgs ...any) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// allow for the all configuration to be loaded first, then allow for the application
		// PostLoad() to run, allowing the setup of resources (logger, bus, ui, etc.) and run user initializers
		// as early as possible before the final configuration is logged. This allows for a couple things:
		// 1. user initializers to account for taking action before logging the final configuration (such as log redactions).
		// 2. other user-facing PostLoad() functions to be able to use the logger, bus, etc. as early as possible. (though it's up to the caller on how these objects are made accessible)

		allConfigs := []any{&a.state.Config}     // process the core application configurations first (logging and development)
		allConfigs = append(allConfigs, a)       // enables application.PostLoad() to be called, initializing all state (bus, logger, ui, etc.)
		allConfigs = append(allConfigs, cfgs...) // allow for all other configs to be loaded + call PostLoad()
		allConfigs = nonNil(allConfigs...)

		if err := fangs.Load(a.setup.FangsConfig, cmd, allConfigs...); err != nil {
			return fmt.Errorf("invalid application config: %v", err)
		}

		// show the app version and configuration...
		logVersion(a.setup, a.state.Logger)

		logConfiguration(a.state.Logger, allConfigs...)

		return nil
	}
}

func (a *application) Run(fn func(ctx context.Context) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		return a.run(ctx, async(ctx, fn))
	}
}

func (a *application) run(ctx context.Context, errs <-chan error) error {
	if a.state.Config.Dev != nil {
		switch a.state.Config.Dev.Profile {
		case ProfileCPU:
			defer profile.Start(profile.CPUProfile).Stop()
		case ProfileMem:
			defer profile.Start(profile.MemProfile).Stop()
		}
	}

	return eventloop(
		ctx,
		a.state.Logger.Nested("component", "eventloop"),
		a.state.Subscription,
		errs,
		a.state.UIs...,
	)
}

func logVersion(cfg SetupConfig, log logger.Logger) {
	if cfg.ID.Version == "" {
		log.Infof(cfg.ID.Name)
		return
	}
	log.Infof(
		"%s version: %+v",
		cfg.ID.Name,
		cfg.ID.Version,
	)
}

func logConfiguration(log logger.Logger, cfgs ...any) {
	var sb strings.Builder

	for _, cfg := range cfgs {
		if cfg == nil {
			continue
		}

		var str string
		if stringer, ok := cfg.(fmt.Stringer); ok {
			str = stringer.String()
		} else {
			// yaml is pretty human friendly (at least when compared to json)
			cfgBytes, err := yaml.Marshal(cfg)
			if err != nil {
				str = fmt.Sprintf("%+v", err)
			} else {
				str = string(cfgBytes)
			}
		}

		str = strings.TrimSpace(str)

		if str != "" && str != "{}" {
			sb.WriteString(str + "\n")
		}
	}

	content := sb.String()

	if content != "" {
		formatted := color.Magenta.Sprint(indent.String("  ", strings.TrimSpace(content)))
		log.Debugf("config:\n%+v", formatted)
	} else {
		log.Debug("config: (none)")
	}
}

func (a *application) SetupCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	return a.setupCommand(cmd, cmd.Flags(), &cmd.PreRunE, cfgs...)
}

func (a *application) setupCommand(cmd *cobra.Command, flags *pflag.FlagSet, fn *func(cmd *cobra.Command, args []string) error, cfgs ...any) *cobra.Command {
	original := *fn
	*fn = func(cmd *cobra.Command, args []string) error {
		err := a.Setup(cfgs...)(cmd, args)
		if err != nil {
			return err
		}
		if original != nil {
			return original(cmd, args)
		}
		return nil
	}

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	a.state.Config.FromCommands = append(a.state.Config.FromCommands, cfgs...)

	fangs.AddFlags(a.setup.FangsConfig.Logger, flags, cfgs...)

	return cmd
}

func (a *application) summarizeConfig(cmd *cobra.Command) string {
	cfg := a.setup.FangsConfig

	summary := "Application Configuration:\n\n"
	summary += indent.String("  ", strings.TrimSuffix(fangs.SummarizeCommand(cfg, cmd, a.state.Config.FromCommands...), "\n"))
	summary += "\n"
	summary += "Config Search Locations:\n"
	for _, f := range fangs.SummarizeLocations(cfg) {
		if !strings.HasSuffix(f, ".yaml") {
			continue
		}
		summary += "  - " + f + "\n"
	}
	return strings.TrimSpace(summary)
}

func setupRootCmd(a *application) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "",
		Version: a.setup.ID.Version,
	}

	var helpUsageTemplate = fmt.Sprintf(`{{if (or .Long .Short)}}{{.Long}}{{if not .Long}}{{.Short}}{{end}}

{{end}}Usage:{{if (and .Runnable (ne .CommandPath "%s"))}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if .HasExample}}

{{.Example}}{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{if not .CommandPath}}Global {{end}}Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if (and .HasAvailableInheritedFlags (not .CommandPath))}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{if .CommandPath}}{{.CommandPath}} {{end}}[command] --help" for more information about a command.{{end}}
`, a.setup.ID.Name)

	cmd.SetUsageTemplate(helpUsageTemplate)
	cmd.SetHelpTemplate(helpUsageTemplate)

	cmd.SetVersionTemplate(fmt.Sprintf("%s {{.Version}}\n", a.setup.ID.Name))

	helpFn := cmd.HelpFunc()
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		// root.Example is set _after all added commands_ because it collects all the
		// options structs in order to output an accurate "config file" summary
		cmd.Example = a.summarizeConfig(cmd)
		helpFn(cmd, args)
	})

	return a.setupCommand(cmd, cmd.PersistentFlags(), &cmd.PreRunE, &a.setup)
}

func async(ctx context.Context, f func(ctx context.Context) error) <-chan error {
	errs := make(chan error)
	go func() {
		defer close(errs)
		if err := f(ctx); err != nil {
			errs <- err
		}
	}()
	return errs
}
