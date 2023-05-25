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

func Test_Application_Setup(t *testing.T) {
	name := "puppy"
	version := "2.0"

	type Embedded struct {
		FreeBed string `yaml:"bed"`
	}

	type EmbeddedInline struct {
		InlineBed string `yaml:"inline-bed"`
	}

	type Nested struct {
		Stuff string `yaml:"stuff"`
	}

	type CmdConfig struct {
		Name           string `yaml:"name"`
		Thing          Nested `yaml:"thing"`
		EmbeddedInline `yaml:",inline"`
		Embedded       `yaml:"embedded"`
	}

	defaultCmdCfg := func() *CmdConfig {
		return &CmdConfig{
			Name: "name!",
			Thing: Nested{
				Stuff: "stuff!",
			},
			EmbeddedInline: EmbeddedInline{
				InlineBed: "inline bed!",
			},
			Embedded: Embedded{
				FreeBed: "free bed!",
			},
		}
	}

	tests := []struct {
		name      string
		cmdCfg    any
		cfg       *Config
		assertRun func(cmd *cobra.Command, args []string)
		assertCfg func(cfg *CmdConfig)
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name:   "reads configuration (fangs is wired)",
			cmdCfg: defaultCmdCfg(),
			cfg:    NewConfig(name, version),
			assertCfg: func(cfg *CmdConfig) {
				assert.Equal(t, &CmdConfig{
					Name: "name!",
					Thing: Nested{
						Stuff: "ruff-ruff!", // note the override
					},
					EmbeddedInline: EmbeddedInline{
						InlineBed: "inline bed!",
					},
					Embedded: Embedded{
						FreeBed: "free bed!",
					},
				}, cfg)
			},
		},
		{
			name: "missing command config does not panic",
			cfg:  NewConfig(name, version),
		},
		{
			name: "runs initializers",
			cfg: NewConfig(name, version).WithInitializers(
				func(cfg Config, state State) error {
					t.Setenv("PUPPY_THING_STUFF", "bark-bark!")
					return nil
				},
			),
			assertRun: func(cmd *cobra.Command, args []string) {
				assert.Equal(t, "bark-bark!", os.Getenv("PUPPY_THING_STUFF"))
			},
		},
		{
			name: "can configure a logger",
			cfg: NewConfig(name, version).
				WithLoggingConfig(LoggingConfig{Level: logger.InfoLevel}).
				WithInitializers(
					func(cfg Config, state State) error {
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
						return nil
					},
				),
		},
		{
			// TODO: missing bus constructor from this test
			name: "wires up state via passed constructors",
			cfg: NewConfig(name, version).
				WithUIConstructor(func(config Config) ([]UI, error) {
					return []UI{&mockUI{}}, nil
				}).
				WithLoggerConstructor(func(config Config) (logger.Logger, error) {
					return newMockLogger(), nil
				}).WithInitializers(
				func(cfg Config, state State) error {
					require.NotNil(t, state.Logger)
					_, ok := state.Logger.(*mockLogger)
					assert.True(t, ok, "expected logger to be a mock")

					require.NotEmpty(t, state.UIs)
					_, ok = state.UIs[0].(*mockUI)
					assert.True(t, ok, "expected UI to be a mock")

					return nil
				}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PUPPY_THING_STUFF", "ruff-ruff!")

			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}

			if tt.assertRun == nil {
				tt.assertRun = func(cmd *cobra.Command, args []string) {
					// no-op
				}
			}

			app := New(*tt.cfg)

			cmd := &cobra.Command{
				DisableFlagParsing: true,
				Args:               cobra.ArbitraryArgs,
				PreRunE:            app.Setup(tt.cmdCfg),
				Run:                tt.assertRun,
			}

			tt.wantErr(t, cmd.Execute())
			if tt.assertCfg != nil {
				tt.assertCfg(tt.cmdCfg.(*CmdConfig))
			}
		})
	}
}

func Test_SetupCommand(t *testing.T) {
	p := &persistent{}

	persistentPreRunCalled := false
	root := &cobra.Command{
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			persistentPreRunCalled = true
			return nil
		},
	}

	a := New(Config{
		Name:        "myApp",
		Version:     "v2.4.11",
		FangsConfig: fangs.NewConfig("myApp"),
	})

	root = a.SetupPersistentCommand(root, p)

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
	assert.Contains(t, usage, "--verbosity")
	assert.Contains(t, usage, "--output")
	assert.Contains(t, usage, "--extras")
	assert.Contains(t, usage, "--online")

	err := root.PersistentPreRunE(sub, []string{})
	require.NoError(t, err)

	err = sub.PreRunE(sub, []string{})
	require.NoError(t, err)

	assert.True(t, persistentPreRunCalled)
	assert.True(t, preRunCalled)
}

type persistent struct {
	Config    string
	Verbosity int
}

var _ fangs.FlagAdder = (*persistent)(nil)

func (t *persistent) AddFlags(flags fangs.FlagSet) {
	flags.StringVarP(&t.Config, "config", "c", "the persistent config")
	flags.CountVarP(&t.Verbosity, "verbosity", "v", "the persistent verbosity")
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
