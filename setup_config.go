package clio

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wagoodman/go-partybus"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
	"github.com/anchore/go-logger/adapter/redact"
)

type SetupConfig struct {
	// Metadata about the target application
	ID Identification

	// Default configuration items that end up in the target application configuration
	DefaultLoggingConfig     *LoggingConfig
	DefaultDevelopmentConfig *DevelopmentConfig

	// Items required for setting up the application (clio-only configuration)
	FangsConfig       fangs.Config
	BusConstructor    BusConstructor
	LoggerConstructor LoggerConstructor
	UIConstructor     UIConstructor
	Initializers      []Initializer
	postConstructs    []postConstruct
	postRuns          []PostRun
}

func NewSetupConfig(id Identification) *SetupConfig {
	return &SetupConfig{
		ID:                id,
		LoggerConstructor: DefaultLogger,
		BusConstructor:    newBus,
		UIConstructor:     newUI,
		FangsConfig:       fangs.NewConfig(id.Name).WithConfigEnvVar(),
		DefaultLoggingConfig: &LoggingConfig{
			Level: logger.WarnLevel,
		},
		// note: no ui selector or dev options by default...
	}
}

func (c *SetupConfig) WithUI(uis ...UI) *SetupConfig {
	c.UIConstructor = func(cfg Config) ([]UI, error) {
		return uis, nil
	}
	return c
}

func (c *SetupConfig) WithUIConstructor(constructor UIConstructor) *SetupConfig {
	c.UIConstructor = constructor
	return c
}

func (c *SetupConfig) WithBusConstructor(constructor BusConstructor) *SetupConfig {
	c.BusConstructor = constructor
	return c
}

func (c *SetupConfig) WithNoBus() *SetupConfig {
	c.BusConstructor = func(config Config) *partybus.Bus {
		return nil
	}
	return c
}

func (c *SetupConfig) WithLoggerConstructor(constructor LoggerConstructor) *SetupConfig {
	c.LoggerConstructor = constructor
	return c
}

func (c *SetupConfig) WithConfigFinders(finders ...fangs.Finder) *SetupConfig {
	c.FangsConfig.Finders = append(c.FangsConfig.Finders, finders...)
	return c
}

func (c *SetupConfig) WithConfigProfile() *SetupConfig {
	c.FangsConfig = c.FangsConfig.WithProfileEnvVar()
	c.FangsConfig.Finders = append([]fangs.Finder{func(cfg fangs.Config) ([]string, error) {
		if cfg.Profile == "" {
			return nil, nil
		}
		file := filepath.Join("."+cfg.AppName, cfg.Profile+".yaml")

		// if path does not exist, return error
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", file)
		}

		return []string{file}, nil
	}}, c.FangsConfig.Finders...)

	c.Initializers = append(c.Initializers, func(state *State) error {
		if c.FangsConfig.Profile != "" {
			state.Logger.Infof("Using config profile: %q", c.FangsConfig.Profile)
		}
		return nil
	})
	return c
}

func (c *SetupConfig) WithDevelopmentConfig(cfg DevelopmentConfig) *SetupConfig {
	c.DefaultDevelopmentConfig = &cfg
	return c
}

func (c *SetupConfig) WithLoggingConfig(cfg LoggingConfig) *SetupConfig {
	c.DefaultLoggingConfig = &cfg
	return c
}

func (c *SetupConfig) WithNoLogging() *SetupConfig {
	c.DefaultLoggingConfig = nil
	c.LoggerConstructor = func(_ Config, _ redact.Store) (logger.Logger, error) {
		return discard.New(), nil
	}
	return c
}

func (c *SetupConfig) WithInitializers(initializers ...Initializer) *SetupConfig {
	c.Initializers = append(c.Initializers, initializers...)
	return c
}

func (c *SetupConfig) WithPostRuns(postRuns ...PostRun) *SetupConfig {
	c.postRuns = append(c.postRuns, postRuns...)
	return c
}

func (c *SetupConfig) withPostConstructs(postConstructs ...postConstruct) *SetupConfig {
	c.postConstructs = append(c.postConstructs, postConstructs...)
	return c
}

// WithGlobalConfigFlag adds the global `-c` / `--config` flags to the root command
func (c *SetupConfig) WithGlobalConfigFlag() *SetupConfig {
	return c.withPostConstructs(func(a *application) {
		a.AddFlags(a.root.PersistentFlags(), &a.setupConfig.FangsConfig)
	})
}

// WithGlobalLoggingFlags adds the global logging flags to the root command.
func (c *SetupConfig) WithGlobalLoggingFlags() *SetupConfig {
	return c.withPostConstructs(func(a *application) {
		a.AddFlags(a.root.PersistentFlags(), &a.state.Config)
	})
}

func (c *SetupConfig) WithConfigInRootHelp() *SetupConfig {
	return c
}
