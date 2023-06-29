package clio

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wagoodman/go-partybus"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
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

var _ logger.Logger = (*mockLogger)(nil)

type mockLogger struct {
	logger.Logger
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		Logger: discard.New(),
	}
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

	cmd := &cobra.Command{
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			assert.Equal(t, "ruff-ruff!", os.Getenv("PUPPY_THING_STUFF"))
		},
	}

	_ = New(*cfg, cmd, cmdCfg)

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

	cfg := NewSetupConfig(Identification{Name: name, Version: version}).
		WithUI(&mockUI{}).
		WithLoggerConstructor(func(config Config) (logger.Logger, error) {
			return newMockLogger(), nil
		})

	cmd := &cobra.Command{
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Run:                func(cmd *cobra.Command, args []string) {},
	}

	app := New(*cfg, cmd)

	require.NoError(t, cmd.Execute())
	state := app.(*application).State()

	require.NotNil(t, state.Logger)
	_, ok := state.Logger.(*mockLogger)
	assert.True(t, ok, "expected logger to be a mock")

	require.NotEmpty(t, state.UIs)
	_, ok = state.UIs[0].(*mockUI)
	assert.True(t, ok, "expected UI to be a mock")

	// TODO: missing bus constructor from this test
}

func Test_Application_Setup_ConfigureLogger(t *testing.T) {
	name := "puppy"
	version := "2.0"

	cfg := NewSetupConfig(Identification{Name: name, Version: version}).
		WithLoggingConfig(LoggingConfig{Level: logger.InfoLevel})

	cmd := &cobra.Command{
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Run:                func(cmd *cobra.Command, args []string) {},
	}

	app := New(*cfg, cmd)

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

	app := New(*cfg, &cobra.Command{})

	cmd := app.SetupCommand(
		&cobra.Command{
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			Run: func(cmd *cobra.Command, args []string) {
				assert.Equal(t, "bark-bark!", os.Getenv("PUPPY_THING_STUFF"))
			},
		})

	require.NoError(t, cmd.Execute())
}

func Test_SetupCommand(t *testing.T) {
	p := &persistent{}

	root := &cobra.Command{}

	cfg := NewSetupConfig(Identification{Name: "myApp", Version: "v2.4.11"}).
		WithConfigInRootHelp().
		WithGlobalConfigFlag().
		WithGlobalLoggingFlags()

	a := New(*cfg, root)

	a.AddFlags(root.PersistentFlags(), p)

	preRunCalled := false
	sub := &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) error {
			preRunCalled = true
			return nil
		},
	}

	f := &f1{}
	sub = a.SetupCommand(sub, f)

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
