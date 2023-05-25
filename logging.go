package clio

import (
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/term"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
	"github.com/anchore/go-logger/adapter/logrus"
)

type terminalDetector interface {
	StdoutIsTerminal() bool
	StderrIsTerminal() bool
}

type stockTerminalDetector struct{}

func (s stockTerminalDetector) StdoutIsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func (s stockTerminalDetector) StderrIsTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

type LoggerConstructor func(*Config) (logger.Logger, error)

func DefaultLogger(clioCfg *Config) (logger.Logger, error) {
	cfg := clioCfg.Log
	if cfg == nil {
		return discard.New(), nil
	}

	l, err := logrus.New(
		logrus.Config{
			EnableConsole: cfg.Verbosity > 0 && !cfg.Quiet,
			FileLocation:  cfg.FileLocation,
			Level:         cfg.Level,
		},
	)
	if err != nil {
		return nil, err
	}

	return l, nil
}

var _ LoggerConstructor = DefaultLogger

// LoggingConfig contains all logging-related configuration options available to the user via the application config.
type LoggingConfig struct {
	Quiet        bool         `yaml:"quiet" json:"quiet"` // -q, indicates to not show any status output to stderr
	Verbosity    int          `yaml:"-" json:"-" `        // -v or -vv , controlling which UI (ETUI vs logging) and what the log level should be
	Level        logger.Level `yaml:"level" json:"level"` // the log level string hint
	FileLocation string       `yaml:"file" json:"file"`   // the file path to write logs to

	terminalDetector terminalDetector // for testing

	// not implemented upstream
	// Structured   bool         `yaml:"structured" json:"structured" mapstructure:"structured"`                        // show all log entries as JSON formatted strings
}

var _ fangs.FlagAdder = (*LoggingConfig)(nil)
var _ fangs.PostLoad = (*LoggingConfig)(nil)

func (l *LoggingConfig) PostLoad() error {
	lvl, err := l.selectLevel()
	if err != nil {
		return fmt.Errorf("unable to select logging level: %w", err)
	}

	l.Level = lvl

	return nil
}

func (l *LoggingConfig) selectLevel() (logger.Level, error) {
	if l == nil {
		// since the logger might not exist, we'll stick with a relatively safe default
		return logger.WarnLevel, nil
	}
	switch {
	case l.Quiet:
		// TODO: this is bad: quiet option trumps all other logging options (such as to a file on disk)
		// we should be able to quiet the console logging and leave file logging alone...
		// ... this will be an enhancement for later
		return logger.DisabledLevel, nil

	case l.Verbosity > 0:
		return logger.LevelFromVerbosity(l.Verbosity, logger.WarnLevel, logger.InfoLevel, logger.DebugLevel, logger.TraceLevel), nil

	case l.Level != "":
		var err error
		l.Level, err = logger.LevelFromString(string(l.Level))
		if err != nil {
			return logger.DisabledLevel, err
		}

		if logger.IsVerbose(l.Level) {
			l.Verbosity = 1
		}
	case l.Level == "":
		// note: the logging config exists, so we expect a logger by default
		return logger.InfoLevel, nil
	}
	return l.Level, nil
}

func (l *LoggingConfig) AllowUI(stdin fs.File) bool {
	pipedInput, err := isPipedInput(stdin)
	if err != nil || pipedInput {
		// since we can't tell if there was piped input we assume that there could be to disable the ETUI
		return false
	}

	if l == nil {
		return true
	}

	if l.terminalDetector == nil {
		l.terminalDetector = stockTerminalDetector{}
	}

	isStdoutATty := l.terminalDetector.StdoutIsTerminal()
	isStderrATty := l.terminalDetector.StderrIsTerminal()
	notATerminal := !isStderrATty && !isStdoutATty
	if notATerminal || !isStderrATty {
		// most UIs should be shown on stderr, not out
		return false
	}

	return l.Verbosity == 0
}

// isPipedInput returns true if there is no input device, which means the user **may** be providing input via a pipe.
func isPipedInput(stdin fs.File) (bool, error) {
	if stdin == nil {
		return false, nil
	}

	fi, err := stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("unable to determine if there is piped input: %w", err)
	}

	// note: we should NOT use the absence of a character device here as the hint that there may be input expected
	// on stdin, as running this application as a subprocess you would expect no character device to be present but input can
	// be from either stdin or indicated by the CLI. Checking if stdin is a pipe is the most direct way to determine
	// if there *may* be bytes that will show up on stdin that should be used for the analysis source.
	return fi.Mode()&os.ModeNamedPipe != 0, nil
}

func (l *LoggingConfig) AddFlags(flags fangs.FlagSet) {
	flags.CountVarP(&l.Verbosity, "verbose", "v", "increase verbosity (-v = info, -vv = debug)")
	flags.BoolVarP(&l.Quiet, "quiet", "q", "suppress all logging output")
}
