package clio

import (
	"github.com/wagoodman/go-partybus"

	"github.com/anchore/go-logger"
)

type State struct {
	Config       Config
	Bus          *partybus.Bus
	Subscription *partybus.Subscription
	Logger       logger.Logger
	UIs          []UI
}

type Config struct {
	// Items that end up in the target application configuration
	Log *LoggingConfig     `yaml:"log" json:"log" mapstructure:"log"`
	Dev *DevelopmentConfig `yaml:"dev" json:"dev" mapstructure:"dev"`

	FromCommands []any `yaml:"-" json:"-" mapstructure:"-"`
}

func (s *State) setupLogger(cx LoggerConstructor) error {
	if cx == nil {
		cx = DefaultLogger
	}

	lgr, err := cx(s.Config)
	if err != nil {
		return err
	}

	s.Logger = lgr
	return nil
}

func (s *State) setupBus(cx BusConstructor) {
	if cx == nil {
		cx = newBus
	}
	s.Bus = cx(s.Config)
	if s.Bus != nil {
		s.Subscription = s.Bus.Subscribe()
	}
}

func (s *State) setupUI(cx UIConstructor) error {
	if cx == nil {
		cx = newUI
	}
	var err error
	s.UIs, err = cx(s.Config)
	return err
}
