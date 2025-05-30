package clio

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wagoodman/go-partybus"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
	"github.com/anchore/go-logger/adapter/redact"
)

const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var ansiPattern = regexp.MustCompile(ansi)

func stripAnsi(in string) string {
	return ansiPattern.ReplaceAllString(in, "")
}

func Test_stripAnsi(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no ansi",
			in:   "no ansi",
			want: "no ansi",
		},
		{
			name: "single ansi",
			in:   "single \u001B[31mansi\u001B[0m",
			want: "single ansi",
		},
		{
			name: "multiple ansi",
			in:   "single \u001B[31mansi\u001B[0m and \u001B[32mcolor\u001B[0m",
			want: "single ansi and color",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripAnsi(tt.in))
		})
	}
}

var _ UI = (*mockUI)(nil)

type mockUI struct{}

func (m mockUI) Setup(_ partybus.Unsubscribable) error {
	return nil
}

func (m mockUI) Handle(_ partybus.Event) error {
	return nil
}

func (m mockUI) Teardown(_ bool) error {
	return nil
}

func Test_Application_Setup_WiresFangs(t *testing.T) {
	name := "puppy"
	version := "2.0"

	type EmbeddedConfig struct {
		FreeBed string `yaml:"bed"`
	}

	type EmbeddedInlineConfig struct {
		InlineBed string `yaml:"inline-bed"`
	}

	type NestedConfig struct {
		Stuff string `yaml:"stuff"`
	}

	type CmdConfig struct {
		Name                 string       `yaml:"name"`
		Thing                NestedConfig `yaml:"thing"`
		EmbeddedInlineConfig `yaml:",inline"`
		EmbeddedConfig       `yaml:"embedded"`
	}

	cfg := NewSetupConfig(Identification{Name: name, Version: version})

	cmdCfg := &CmdConfig{
		Name: "name!",
		Thing: NestedConfig{
			Stuff: "stuff!", // value under test...
		},
		EmbeddedInlineConfig: EmbeddedInlineConfig{
			InlineBed: "inline bed!",
		},
		EmbeddedConfig: EmbeddedConfig{
			FreeBed: "free bed!",
		},
	}

	t.Setenv("PUPPY_THING_STUFF", "ruff-ruff!")

	app := New(*cfg)

	cmd := app.SetupRootCommand(&cobra.Command{
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			assert.Equal(t, "ruff-ruff!", os.Getenv("PUPPY_THING_STUFF"))
		},
	}, cmdCfg)

	require.NoError(t, cmd.Execute())

	assert.Equal(t, &CmdConfig{
		Name: "name!",
		Thing: NestedConfig{
			Stuff: "ruff-ruff!", // note the override
		},
		EmbeddedInlineConfig: EmbeddedInlineConfig{
			InlineBed: "inline bed!",
		},
		EmbeddedConfig: EmbeddedConfig{
			FreeBed: "free bed!",
		},
	}, cmdCfg)
}

func Test_Application_Setup_PassLoggerConstructor(t *testing.T) {
	name := "puppy"
	version := "2.0"

	type mockLogger struct {
		logger.Logger
	}

	cfg := NewSetupConfig(Identification{Name: name, Version: version}).
		WithUI(&mockUI{}).
		WithLoggerConstructor(func(_ Config, _ redact.Store) (logger.Logger, error) {
			return &mockLogger{Logger: discard.New()}, nil
		})

	app := New(*cfg)

	cmd := app.SetupRootCommand(&cobra.Command{
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Run:                func(cmd *cobra.Command, args []string) {},
	})

	require.NoError(t, cmd.Execute())
	state := app.(*application).State()

	require.NotNil(t, state.Logger)
	_, ok := state.Logger.(*mockLogger)
	assert.True(t, ok, "expected logger to be a mock")

	require.NotEmpty(t, state.UI)
	_, ok = state.UI.uis[0].(*mockUI)
	assert.True(t, ok, "expected UI to be a mock")

	// TODO: missing bus constructor from this test
}

func Test_Application_Setup_ConfigureLogger(t *testing.T) {
	name := "puppy"
	version := "2.0"

	cfg := NewSetupConfig(Identification{Name: name, Version: version}).
		WithLoggingConfig(LoggingConfig{Level: logger.InfoLevel})

	app := New(*cfg)

	cmd := app.SetupRootCommand(&cobra.Command{
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Run:                func(cmd *cobra.Command, args []string) {},
	})

	require.NoError(t, cmd.Execute())
	state := app.(*application).State()

	require.NotNil(t, state.Logger)
	c, ok := state.Logger.(logger.Controller)
	if !ok {
		t.Fatal("expected logger to be a controller")
	}

	buf := &bytes.Buffer{}
	c.SetOutput(buf)
	state.Logger.Info("test")

	// prove this is a NOT a nil logger
	assert.Equal(t, "[0000]  INFO test\n", stripAnsi(buf.String()))
}

func Test_Application_Setup_RunsInitializers(t *testing.T) {
	name := "puppy"
	version := "2.0"

	cfg := NewSetupConfig(Identification{Name: name, Version: version}).WithInitializers(
		func(state *State) error {
			t.Setenv("PUPPY_THING_STUFF", "bark-bark!")
			return nil
		},
	)

	t.Setenv("PUPPY_THING_STUFF", "ruff-ruff!")

	app := New(*cfg)
	assert.False(t, app.(*application).resourcesLoaded)

	cmd := app.SetupCommand(
		&cobra.Command{
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			Run: func(cmd *cobra.Command, args []string) {
				assert.Equal(t, "bark-bark!", os.Getenv("PUPPY_THING_STUFF"))
			},
		})

	require.NoError(t, cmd.Execute())

	assert.True(t, app.(*application).resourcesLoaded)
}

func Test_Application_Setup_RunsPostRuns(t *testing.T) {
	name := "puppy"
	version := "2.0"

	cfg := NewSetupConfig(Identification{Name: name, Version: version}).WithPostRuns(
		func(state *State, err error) {
			t.Setenv("PUPPY_THING_STUFF", "bark-bark!")
		},
	)

	t.Setenv("PUPPY_THING_STUFF", "ruff-ruff!")

	app := New(*cfg)

	app.SetupRootCommand(
		&cobra.Command{
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return nil
			},
		})

	app.Run()

	assert.Equal(t, "bark-bark!", os.Getenv("PUPPY_THING_STUFF"))
}

func Test_Application_Setup_RunsPostRunsOnError(t *testing.T) {
	name := "puppy"
	version := "2.0"

	cfg := NewSetupConfig(Identification{Name: name, Version: version}).WithPostRuns(
		func(state *State, err error) {
			if err != nil {
				t.Setenv("PUPPY_ERROR", err.Error())
			}
		},
	).WithMapExitCode(func(err error) int {
		return 0 // ensure the test won't exit
	})

	t.Setenv("PUPPY_ERROR", "")

	app := New(*cfg)

	app.SetupRootCommand(
		&cobra.Command{
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return errors.New("bark-bark!")
			},
		})

	app.Run()

	assert.Equal(t, "bark-bark!", os.Getenv("PUPPY_ERROR"))
}

func Test_SetupCommand(t *testing.T) {
	p := &persistent{}

	cfg := NewSetupConfig(Identification{Name: "myApp", Version: "v2.4.11"}).
		WithConfigInRootHelp().
		WithGlobalConfigFlag().
		WithGlobalLoggingFlags()

	app := New(*cfg)

	root := app.SetupRootCommand(&cobra.Command{})

	app.AddFlags(root.PersistentFlags(), p)

	preRunCalled := false
	sub := &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) error {
			preRunCalled = true
			return nil
		},
	}

	f := &f1{}
	sub = app.SetupCommand(sub, f)

	root.AddCommand(sub)

	usage := sub.UsageString()

	assert.Contains(t, usage, "--config")
	assert.Contains(t, usage, "--verbose")
	assert.Contains(t, usage, "--quiet")
	assert.Contains(t, usage, "--persistent-config")
	assert.Contains(t, usage, "--persistent-verbosity")
	assert.Contains(t, usage, "--output")
	assert.Contains(t, usage, "--extras")
	assert.Contains(t, usage, "--online")

	err := sub.PreRunE(sub, []string{})
	require.NoError(t, err)

	assert.True(t, preRunCalled)
}

type persistent struct {
	Config    string
	Verbosity int
}

var _ fangs.FlagAdder = (*persistent)(nil)

func (t *persistent) AddFlags(flags fangs.FlagSet) {
	flags.StringVarP(&t.Config, "persistent-config", "", "the persistent config")
	flags.CountVarP(&t.Verbosity, "persistent-verbosity", "", "the persistent verbosity")
}

type f1 struct {
	Output string
	Extras bool
	Online *bool
}

var _ fangs.FlagAdder = (*f1)(nil)

func (t *f1) AddFlags(flags fangs.FlagSet) {
	flags.StringVarP(&t.Output, "output", "o", "the flag output")
	flags.BoolVarP(&t.Extras, "extras", "", "the flag extras")
	flags.BoolPtrVarP(&t.Online, "online", "", "the flag online")
}

func Test_Run(t *testing.T) {
	runCalled := false
	app := New(*NewSetupConfig(Identification{}).WithNoBus())
	app.SetupRootCommand(&cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			runCalled = true
			return nil
		},
	})
	app.Run()
	require.True(t, runCalled)
}

func Test_RunPanicWithoutRootCommand(t *testing.T) {
	require.PanicsWithError(t, setupRootCommandNotCalledError, func() {
		app := New(*NewSetupConfig(Identification{}).WithNoBus())
		app.Run()
	})
}

func Test_RunExitError(t *testing.T) {
	d := t.TempDir()
	logFile := filepath.Join(d, "log.txt")

	if os.Getenv("CLIO_RUN_EXIT_ERROR") == "YES" {
		app := New(*NewSetupConfig(Identification{}).WithNoBus().WithLoggingConfig(LoggingConfig{
			Level:        logger.InfoLevel,
			FileLocation: os.Getenv("CLIO_LOG_FILE"),
		}))
		app.SetupRootCommand(&cobra.Command{
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("an error occurred")
			},
		})
		app.Run()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=Test_RunExitError")
	cmd.Env = append(os.Environ(), fmt.Sprintf("CLIO_LOG_FILE=%s", logFile), "CLIO_RUN_EXIT_ERROR=YES")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	logContents, readErr := os.ReadFile(logFile)
	require.NoError(t, readErr)

	var e *exec.ExitError
	if errors.As(err, &e) && !e.Success() {
		// ensure that errors are reported to stderr/log, not stdout
		assert.Contains(t, stderr.String(), "an error occurred")
		assert.NotContains(t, stdout.String(), "an error occurred")
		assert.Contains(t, string(logContents), "an error occurred")
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

func Test_Run_InvokesBusExit(t *testing.T) {
	runCalled := false

	var sub *partybus.Subscription

	app := New(*NewSetupConfig(Identification{}).WithInitializers(
		func(state *State) error {
			sub = state.Bus.Subscribe()
			return nil
		},
	))
	app.SetupRootCommand(&cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			runCalled = true
			return nil
		},
	})
	app.Run()
	require.True(t, runCalled)

	events := sub.Events()

	e := <-events

	assert.Equal(t, e.Type, ExitEventType)
}

type mockErrorLogger struct {
	logger.Logger
	msg string
}

func (m *mockErrorLogger) Error(msg ...any) {
	m.msg += fmt.Sprintf("%+v", msg)
}

func TestHandleExitError(t *testing.T) {
	tests := []struct {
		name        string
		app         application
		err         error
		expectLog   bool
		expectPrint bool
	}{
		{
			name: "with logger and resources loaded",
			app: application{
				state: State{
					Logger: &mockErrorLogger{},
				},
				resourcesLoaded: true,
			},
			err:         errors.New("test error"),
			expectLog:   true,
			expectPrint: false,
		},
		{
			name: "with logger but resources not loaded",
			app: application{
				state: State{
					Logger: &mockErrorLogger{},
				},
				resourcesLoaded: false,
			},
			err:         errors.New("test error"),
			expectLog:   false,
			expectPrint: true,
		},
		{
			name: "without logger and resources loaded",
			app: application{
				state:           State{},
				resourcesLoaded: true,
			},
			err:         errors.New("test error"),
			expectLog:   false,
			expectPrint: true,
		},
		{
			name: "without logger and resources not loaded",
			app: application{
				state:           State{},
				resourcesLoaded: false,
			},
			err:         errors.New("test error"),
			expectLog:   false,
			expectPrint: true,
		},
	}

	originalStderr := os.Stderr
	defer func() { os.Stderr = originalStderr }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer

			tt.app.handleExitError(tt.err, &stderr)

			if tt.expectLog {
				assert.Contains(t, tt.app.state.Logger.(*mockErrorLogger).msg, "test error")
			}

			if tt.expectPrint {
				assert.Contains(t, stderr.String(), "test error")
			} else {
				assert.Empty(t, stderr.String())
			}
		})
	}
}
