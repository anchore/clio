package clio

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_ConfigCommandDefaults(t *testing.T) {
	type options struct {
		SomePath string `mapstructure:"some-path"`
		Name     string `mapstructure:"name"`
		Password string `mapstructure:"password"`
	}

	cfg := NewSetupConfig(Identification{
		Name: "my-app",
	})

	cfg.FangsConfig.Files = []string{"testdata/.my-app.yaml"}

	app := New(*cfg)

	// emulate a PostLoad hook which adds values to redact
	app.(*application).State().RedactStore.Add("default-password") // config file values should not be loaded

	userHome, _ := os.UserHomeDir()
	opt := &options{
		SomePath: fmt.Sprintf("%s/some/dir", userHome),
		Name:     "default-name",
		Password: "default-password",
	}
	_ = app.SetupCommand(&cobra.Command{}, opt)

	t.Setenv("MY_APP_NAME", "env-name") // this should not be loaded

	stdout, _ := captureStd(func() {
		configCmd := ConfigCommand(app, nil)
		err := configCmd.RunE(configCmd, nil)
		require.NoError(t, err)
	})

	require.Equal(t, `log:
  # (env: MY_APP_LOG_QUIET)
  quiet: false

  # (env: MY_APP_LOG_VERBOSITY)
  verbosity: 0

  # (env: MY_APP_LOG_LEVEL)
  level: ''

  # (env: MY_APP_LOG_FILE)
  file: ''

dev:
  # (env: MY_APP_DEV_PROFILE)
  profile: ''

# (env: MY_APP_SOME_PATH)
some-path: '~/some/dir'

# (env: MY_APP_NAME)
name: 'default-name'

# (env: MY_APP_PASSWORD)
password: '*******'
`, stdout)
}

func Test_ConfigCommandLoad(t *testing.T) {
	type options struct {
		Name     string `mapstructure:"name"`
		Password string `mapstructure:"password"`
	}

	cfg := NewSetupConfig(Identification{
		Name: "my-app",
	})

	cfg.FangsConfig.Files = []string{"testdata/.my-app.yaml"}

	app := New(*cfg)

	// emulate a PostLoad hook which adds values to redact
	app.(*application).State().RedactStore.Add("password") // default-password is not redacted, only password

	opt := &options{
		Name:     "default-name",
		Password: "default-password",
	}
	_ = app.SetupCommand(&cobra.Command{}, opt)

	t.Setenv("MY_APP_NAME", "env-name") // this should be loaded

	stdout, _ := captureStd(func() {
		configCmd := ConfigCommand(app, nil)
		err := configCmd.Flags().Set("load", "true")
		require.NoError(t, err)
		err = configCmd.RunE(configCmd, nil)
		require.NoError(t, err)
	})

	require.Equal(t, `log:
  # (env: MY_APP_LOG_QUIET)
  quiet: false

  # (env: MY_APP_LOG_VERBOSITY)
  verbosity: 0

  # explicitly set the logging level (available: [error warn info debug trace]) (env: MY_APP_LOG_LEVEL)
  level: 'info'

  # file path to write logs to (env: MY_APP_LOG_FILE)
  file: ''

dev:
  # capture resource profiling data (available: [cpu, mem]) (env: MY_APP_DEV_PROFILE)
  profile: 'none'

# (env: MY_APP_NAME)
name: 'env-name'

# (env: MY_APP_PASSWORD)
password: '*******'
`, stdout)
}

func Test_SummarizeLocationsCommand(t *testing.T) {
	cfg := *NewSetupConfig(Identification{
		Name: "my-app",
	})
	app := New(cfg)

	// should NOT have locations subcommand
	configCmd := ConfigCommand(app, DefaultConfigCommandConfig().WithIncludeLocationsSubcommand(false))
	require.Len(t, configCmd.Commands(), 0)

	// should have locations subcommand
	configCmd = ConfigCommand(app, nil)
	require.Len(t, configCmd.Commands(), 1)

	// should have locations subcommand
	configCmd = ConfigCommand(app, DefaultConfigCommandConfig())
	require.Len(t, configCmd.Commands(), 1)

	cmd := configCmd.Commands()[0]

	stdout, _ := captureStd(func() {
		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})

	require.Contains(t, stdout, ".my-app.yaml")
	require.NotContains(t, stdout, ".my-app.json")
	require.Contains(t, stdout, ".my-app/config.yaml")

	stdout, _ = captureStd(func() {
		_ = cmd.Flags().Set("all", "true")
		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})

	require.Contains(t, stdout, ".my-app.yaml")
	require.Contains(t, stdout, ".my-app.json")
}

func Test_ConsolidateProfileErrors(t *testing.T) {
	tests := []struct {
		file     string
		expected string
	}{
		{
			file:     ".my-app.yaml",
			expected: "not found in any configuration files",
		},
		{
			file:     ".my-app-profiles.yaml",
			expected: "profile not found",
		},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			type options1 struct {
				Name1 string `mapstructure:"name1"`
			}

			type options2 struct {
				Name2 string `mapstructure:"name2"`
			}

			cfg := *NewSetupConfig(Identification{
				Name: "my-app",
			})

			cfg.FangsConfig.Files = []string{filepath.Join("testdata", test.file)}
			cfg.FangsConfig.Profiles = []string{"bad-profile"}
			app := New(cfg)

			// setup multiple commands, which get loaded during config --load
			opt1 := &options1{
				Name1: "default-name1",
			}
			_ = app.SetupCommand(&cobra.Command{}, opt1)

			opt2 := &options2{
				Name2: "default-name2",
			}
			_ = app.SetupCommand(&cobra.Command{}, opt2)

			var err error
			_, _ = captureStd(func() {
				configCmd := ConfigCommand(app, nil)
				err = configCmd.Flags().Set("load", "true")
				require.NoError(t, err)
				err = configCmd.RunE(configCmd, nil)
			})

			// should have errors, but only be found once
			require.Error(t, err)
			require.Equal(t, 1, strings.Count(err.Error(), test.expected))
		})
	}
}

func Test_appendConfigLoadError(t *testing.T) {
	type ty struct{}
	typ := reflect.TypeOf(ty{})
	typErrMsg := fmt.Sprintf("error loading config '%s.%s': ", typ.PkgPath(), typ.Name()) + "%w"

	tests := []struct {
		name     string
		errs     []error
		expected []error
	}{
		{
			name: "no duplicates",
			errs: []error{
				fmt.Errorf("some error"),
				fmt.Errorf("existing error"),
				fmt.Errorf("existing error2"),
				fmt.Errorf("new error"),
			},
			expected: []error{
				fmt.Errorf(typErrMsg, fmt.Errorf("some error")),
				fmt.Errorf(typErrMsg, fmt.Errorf("existing error")),
				fmt.Errorf(typErrMsg, fmt.Errorf("existing error2")),
				fmt.Errorf(typErrMsg, fmt.Errorf("new error")),
			},
		},
		{
			name: "duplicates",
			errs: []error{
				fmt.Errorf("some error"),
				fmt.Errorf("duplicate error"),
				fmt.Errorf("existing error"),
				fmt.Errorf("duplicate error"),
				fmt.Errorf("duplicate error"),
			},
			expected: []error{
				fmt.Errorf(typErrMsg, fmt.Errorf("some error")),
				fmt.Errorf("duplicate error"), // type should be removed and error not duplicated, since multiple types report the same error
				fmt.Errorf(typErrMsg, fmt.Errorf("existing error")),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []error
			for _, err := range test.errs {
				got = appendConfigLoadError(got, typ, err)
			}
			require.ElementsMatch(t, test.expected, got)
		})
	}
}
