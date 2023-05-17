package clio

import (
	"context"
	"fmt"
	"strings"

	"github.com/gookit/color"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"
	"github.com/wagoodman/go-partybus"
	"gopkg.in/yaml.v3"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
)

type Initializer func(cfg Config, state State) error

type State struct {
	Bus          *partybus.Bus
	Subscription *partybus.Subscription
	Logger       logger.Logger
	UIs          []UI
}

type Application interface {
	Setup(cfgs ...any) func(cmd *cobra.Command, args []string) error
	Run(ctx context.Context, errs <-chan error) error
	State() State
}

type application struct {
	config Config
	state  State
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

func (a application) State() State {
	return a.state
}

func (a *application) Setup(cfgs ...any) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		var allConfigs []any
		allConfigs = append(allConfigs, a.config.AdditionalConfigs...)
		allConfigs = append(allConfigs, cfgs...)
		allConfigs = nonNil(allConfigs...)

		if err := fangs.Load(a.config.FangsConfig, cmd, allConfigs...); err != nil {
			return fmt.Errorf("invalid application config: %v", err)
		}

		if a.config.Log != nil {
			if err := fangs.LoadAt(a.config.FangsConfig, cmd, "log", a.config.Log); err != nil {
				return fmt.Errorf("invalid log config: %v", err)
			}
			allConfigs = append(allConfigs, map[string]any{"log": a.config.Log})
		}

		if a.config.Dev != nil {
			if err := fangs.LoadAt(a.config.FangsConfig, cmd, "dev", a.config.Dev); err != nil {
				return fmt.Errorf("invalid dev config: %v", err)
			}
			allConfigs = append(allConfigs, map[string]any{"dev": a.config.Dev})
		}

		if err := a.setupLogger(allConfigs...); err != nil {
			return fmt.Errorf("unable to setup logger: %w", err)
		}

		a.setupBus()

		if err := a.setupUI(); err != nil {
			return fmt.Errorf("unable to setup UI: %w", err)
		}

		for _, init := range a.config.Initializers {
			if err := init(a.config, a.state); err != nil {
				return err
			}
		}

		return nil
	}
}

func (a application) Run(ctx context.Context, errs <-chan error) error {
	if a.config.Dev != nil {
		switch a.config.Dev.Profile {
		case CPUProfile:
			defer profile.Start(profile.CPUProfile).Stop()
		case MemProfile:
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

func (a *application) setupLogger(allConfigs ...any) error {
	cx := a.config.LoggerConstructor
	if cx == nil {
		cx = newLogger
	}

	lgr, err := cx(a.config)
	if err != nil {
		return err
	}

	a.state.Logger = lgr

	// show the app version and configuration...
	logVersion(a.config, a.state.Logger)
	logConfiguration(a.state.Logger, allConfigs...)
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

	for i, cfg := range cfgs {
		if cfg == nil {
			continue
		}

		var str string
		if stringer, ok := cfg.(fmt.Stringer); ok {
			str = stringer.String()
		} else {
			// yaml is pretty human friendly (at least when compared to json)
			cfgBytes, err := yaml.Marshal(&cfgs[i])
			if err != nil {
				str = fmt.Sprintf("%+v", str)
			} else {
				str = string(cfgBytes)
			}
		}

		if str != "" {
			sb.WriteString(str)
		}
	}

	content := sb.String()

	if content != "" {
		formatted := color.Magenta.Sprint(indent(strings.TrimSpace(content), "  "))
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

func indent(text, indent string) string {
	if indent == "" {
		return text
	}
	if len(strings.TrimSpace(text)) == 0 {
		return indent
	}
	if text[len(text)-1:] == "\n" {
		result := ""
		for _, j := range strings.Split(text[:len(text)-1], "\n") {
			result += indent + j + "\n"
		}
		return result
	}
	result := ""
	for _, j := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		result += indent + j + "\n"
	}
	return result[:len(result)-1]
}
