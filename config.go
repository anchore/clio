package clio

import (
	"github.com/spf13/pflag"
	"github.com/wagoodman/go-partybus"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
)

type Config struct {
	Name    string
	Version string

	AdditionalConfigs []interface{}
	Dev               *DevelopmentConfig
	Log               *LoggingConfig

	FangsConfig   fangs.Config
	ConfigFinders []fangs.Finder

	BusConstructor    BusConstructor
	LoggerConstructor LoggerConstructor
	UIConstructor     UIConstructor

	Initializers []Initializer
}

func NewConfig(name, version string) *Config {
	return &Config{
		Name:    name,
		Version: version,
		ConfigFinders: []fangs.Finder{
			fangs.FindDirect,
			fangs.FindInCwd,
			fangs.FindInAppNameSubdir,
			fangs.FindInHomeDir,
			fangs.FindInXDG,
		},
		LoggerConstructor: DefaultLogger,
		BusConstructor:    newBus,
		UIConstructor:     newUI,
		FangsConfig:       fangs.NewConfig(name),
		Log: &LoggingConfig{
			Level: logger.InfoLevel,
		},
		// note: no ui selector or dev options by default...
	}
}

func (c Config) AddFlags(flags *pflag.FlagSet) {
	if c.Log != nil {
		c.Log.AddFlags(flags)
	}
	c.FangsConfig.AddFlags(flags)
}

func (c *Config) WithUIConstructor(constructor UIConstructor) *Config {
	c.UIConstructor = constructor
	return c
}

func (c *Config) WithBusConstructor(constructor BusConstructor) *Config {
	c.BusConstructor = constructor
	return c
}

func (c *Config) WithNoBus() *Config {
	c.BusConstructor = func(config Config) *partybus.Bus {
		return nil
	}
	return c
}

func (c *Config) WithLoggerConstructor(constructor LoggerConstructor) *Config {
	c.LoggerConstructor = constructor
	return c
}

func (c *Config) WithConfigFinders(finders ...fangs.Finder) *Config {
	c.ConfigFinders = append(c.ConfigFinders, finders...)
	return c
}

func (c *Config) WithConfigs(cfg ...any) *Config {
	c.AdditionalConfigs = append(c.AdditionalConfigs, cfg...)
	return c
}

func (c *Config) WithDevelopmentConfig(cfg DevelopmentConfig) *Config {
	c.Dev = &cfg
	return c
}

func (c *Config) WithLoggingConfig(cfg LoggingConfig) *Config {
	c.Log = &cfg
	return c
}

func (c *Config) WithNoLogging() *Config {
	c.Log = nil
	c.LoggerConstructor = func(config Config) (logger.Logger, error) {
		return discard.New(), nil
	}
	return c
}

func (c *Config) WithInitializers(initializers ...Initializer) *Config {
	c.Initializers = append(c.Initializers, initializers...)
	return c
}
