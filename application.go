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
	SetupCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command
}

type application struct {
	root        *cobra.Command
	setupConfig SetupConfig `yaml:"-" mapstructure:"-"`
	state       State       `yaml:"-" mapstructure:"-"`
}

func New(cfg SetupConfig, root *cobra.Command, cfgs ...any) Application {
	a := &application{
		root:        root,
		setupConfig: cfg,
	}

	a.setupRootCommand(root, cfgs...)

	return a
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

// State returns all application configuration and resources to be either used or replaced by the caller. Note: this is only valid after the application has been setup (cobra PreRunE has run).
func (a *application) State() *State {
	return &a.state
}

func (a *application) PostLoad() error {
	if err := a.state.setup(a.setupConfig); err != nil {
		return err
	}

	for _, init := range a.setupConfig.Initializers {
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
		allConfigs, err := a.loadConfigs(cmd, true, cfgs...)
		if err != nil {
			return err
		}

		// show the app version and configuration...
		logVersion(a.setupConfig, a.state.Logger)

		logConfiguration(a.state.Logger, allConfigs...)

		return nil
	}
}

func (a *application) loadConfigs(cmd *cobra.Command, withResources bool, cfgs ...any) ([]any, error) {
	allConfigs := []any{&a.state.Config} // 1. process the core application configurations first (logging and development)
	if withResources {
		allConfigs = append(allConfigs, a) // 2. enables application.PostLoad() to be called, initializing all state (bus, logger, ui, etc.)
	}
	allConfigs = append(allConfigs, cfgs...) // 3. allow for all other configs to be loaded + call PostLoad()
	allConfigs = nonNil(allConfigs...)

	if err := fangs.Load(a.setupConfig.FangsConfig, cmd, allConfigs...); err != nil {
		return nil, fmt.Errorf("invalid application config: %v", err)
	}
	return allConfigs, nil
}

func (a *application) Run(fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return a.run(cmd.Context(), async(cmd, args, fn))
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

func (a *application) setupRootCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	// TODO: more? if doesn't start with name
	if cmd.Use == "" {
		cmd.Use = a.setupConfig.ID.Name
	}

	cmd.Version = a.setupConfig.ID.Version

	cmd.SetVersionTemplate(fmt.Sprintf("%s {{.Version}}\n", a.setupConfig.ID.Name))

	if a.setupConfig.ShowConfigInRootHelp {
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
`, a.setupConfig.ID.Name)

		cmd.SetUsageTemplate(helpUsageTemplate)
		cmd.SetHelpTemplate(helpUsageTemplate)

		helpFn := cmd.HelpFunc()
		cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
			// TODO: I'm still busted... not loading the config (either defaults or current)

			// root.Example is set _after all added commands_ because it collects all the
			// options structs in order to output an accurate "config file" summary
			// note: since all commands tend to share help functions it's important to only patch the example
			// when there is no parent command (i.e. the root command).
			if cmd == a.root {
				_, err := a.loadConfigs(cmd, false, cfgs...)
				if err == nil {
					cmd.Example = a.summarizeConfig(cmd)
				}
			}
			helpFn(cmd, args)
		})
	}

	a.state.Config.Log = a.setupConfig.DefaultLoggingConfig
	a.state.Config.Dev = a.setupConfig.DefaultDevelopmentConfig

	logger := a.setupConfig.FangsConfig.Logger

	if a.setupConfig.LoggingFlags {
		fangs.AddFlags(logger, cmd.PersistentFlags(), &a.state.Config)
	}

	if a.setupConfig.ApplicationConfigPathFlag {
		fangs.AddFlags(logger, cmd.PersistentFlags(), &a.setupConfig.FangsConfig)
	}

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

	if cmd.RunE != nil {
		cmd.RunE = a.Run(cmd.RunE)
	}

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	a.state.Config.FromCommands = append(a.state.Config.FromCommands, cfgs...)

	fangs.AddFlags(a.setupConfig.FangsConfig.Logger, flags, cfgs...)

	return cmd
}

func (a *application) summarizeConfig(cmd *cobra.Command) string {
	cfg := a.setupConfig.FangsConfig

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

func async(cmd *cobra.Command, args []string, f func(cmd *cobra.Command, args []string) error) <-chan error {
	errs := make(chan error)
	go func() {
		defer close(errs)
		if err := f(cmd, args); err != nil {
			errs <- err
		}
	}()
	return errs
}
