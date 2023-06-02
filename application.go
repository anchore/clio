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
	"github.com/wagoodman/go-partybus"
	"gopkg.in/yaml.v3"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
)

type Initializer func(cfg Config, state *State) error

type State struct {
	Bus          *partybus.Bus
	Subscription *partybus.Subscription
	Logger       logger.Logger
	UIs          []UI
}

type Application interface {
	AddConfigs(cfgs ...any)
	Config() Config
	Run(ctx context.Context, errs <-chan error) error
	Setup(cfgs ...any) func(cmd *cobra.Command, args []string) error
	SetupCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command
	SetupPersistentCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command
	State() State
	SummarizeConfig(cmd *cobra.Command) string
}

type application struct {
	configs []any  `yaml:"-" mapstructure:"-"`
	config  Config `yaml:"-" mapstructure:"-"`
	state   State  `yaml:"-" mapstructure:"-"`
}

func New(cfg Config) Application {
	return &application{
		config: cfg,
	}
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

func (a *application) AddConfigs(cfgs ...any) {
	a.configs = append(a.configs, cfgs...)
}

func (a application) Config() Config {
	return a.config
}

func (a application) State() State {
	return a.state
}

func (a *application) PostLoad() error {
	a.setupBus()

	if err := a.setupLogger(); err != nil {
		return fmt.Errorf("unable to setup logger: %w", err)
	}

	if err := a.setupUI(); err != nil {
		return fmt.Errorf("unable to setup UI: %w", err)
	}

	for _, init := range a.config.Initializers {
		if err := init(a.config, &a.state); err != nil {
			return err
		}
	}
	return nil
}

func (a *application) Setup(cfgs ...any) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// allow for the all configuration to be loaded first, then allow for the application
		// PostLoad() to run, allowing the setup of resources (logger, bus, ui, etc.) and run user initializers
		// as early as possible before the final configuration is logged. This allows for a couple things:
		// 1. user initializers to account for taking action before logging the final configuration (such as log redactions).
		// 2. other user-facing PostLoad() functions to be able to use the logger, bus, etc. as early as possible. (though it's up to the caller on how these objects are made accessible)

		allConfigs := []any{&a.config}                                 // process the core application configurations first (logging and development)
		allConfigs = append(allConfigs, a)                             // enables application.PostLoad() to be called, initializing all state (bus, logger, ui, etc.)
		allConfigs = append(allConfigs, a.config.AdditionalConfigs...) // allow for all other configs to be loaded + call PostLoad()
		allConfigs = append(allConfigs, cfgs...)
		allConfigs = nonNil(allConfigs...)

		if err := fangs.Load(a.config.FangsConfig, cmd, allConfigs...); err != nil {
			return fmt.Errorf("invalid application config: %v", err)
		}

		// show the app version and configuration...
		logVersion(a.config, a.state.Logger)

		logConfiguration(a.state.Logger, allConfigs...)

		return nil
	}
}

func (a application) Run(ctx context.Context, errs <-chan error) error {
	if a.config.Dev != nil {
		switch a.config.Dev.Profile {
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

func (a *application) setupLogger() error {
	cx := a.config.LoggerConstructor
	if cx == nil {
		cx = DefaultLogger
	}

	lgr, err := cx(a.config)
	if err != nil {
		return err
	}

	a.state.Logger = lgr
	return nil
}

func logVersion(cfg Config, log logger.Logger) {
	if cfg.Version == "" {
		log.Infof(cfg.Name)
		return
	}
	log.Infof(
		"%s version: %+v",
		cfg.Name,
		cfg.Version,
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

func (a *application) setupBus() {
	cx := a.config.BusConstructor
	if cx == nil {
		cx = newBus
	}
	a.state.Bus = cx(a.config)
	if a.state.Bus != nil {
		a.state.Subscription = a.state.Bus.Subscribe()
	}
}

func (a *application) setupUI() error {
	cx := a.config.UIConstructor
	if cx == nil {
		cx = newUI
	}
	var err error
	a.state.UIs, err = cx(a.config)
	return err
}

func (a *application) SetupCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	return a.setupCommand(cmd, cmd.Flags(), &cmd.PreRunE, cfgs...)
}

func (a *application) SetupPersistentCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	return a.setupCommand(cmd, cmd.PersistentFlags(), &cmd.PreRunE, cfgs...)
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

	a.AddConfigs(cfgs...)

	fangs.AddFlags(a.config.FangsConfig.Logger, flags, cfgs...)

	return cmd
}

func (a *application) SummarizeConfig(cmd *cobra.Command) string {
	cfg := a.config.FangsConfig

	summary := "Application Configuration:\n\n"
	summary += indent.String("  ", strings.TrimSuffix(fangs.SummarizeCommand(cfg, cmd, a.configs...), "\n"))
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
