package clio

import (
	"fmt"
	"testing"

	"github.com/mitchellh/go-homedir"
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

	cfg.FangsConfig.File = "testdata/.my-app.yaml"

	app := New(*cfg)

	// emulate a PostLoad hook which adds values to redact
	app.(*application).State().RedactStore.Add("default-password") // config file values should not be loaded

	userHome, _ := homedir.Dir()
	opt := &options{
		SomePath: fmt.Sprintf("%s/some/dir", userHome),
		Name:     "default-name",
		Password: "default-password",
	}
	_ = app.SetupCommand(&cobra.Command{}, opt)

	t.Setenv("MY_APP_NAME", "env-name") // this should not be loaded

	stdout, _ := captureStd(func() {
		configCmd := ConfigCommand(app, ReplaceHomeDirWithTilde)
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

	cfg.FangsConfig.File = "testdata/.my-app.yaml"

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
		configCmd := ConfigCommand(app, ReplaceHomeDirWithTilde)
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
	configCmd := ConfigCommand(app)
	require.Len(t, configCmd.Commands(), 0)

	// should have locations subcommand
	configCmd = ConfigCommand(app, IncludeLocationsSubcommand)
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
