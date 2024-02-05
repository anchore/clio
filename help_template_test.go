package clio

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger/adapter/redact"
)

// regression test for https://github.com/anchore/clio/issues/40
func Test_conflictingConfigErrors(t *testing.T) {
	fangsConfig := fangs.NewConfig("test")
	fangsConfig.File = "testdata/conflicting_config.yaml"
	application := &application{
		root: &cobra.Command{},
		state: State{
			Config: Config{
				FromCommands: []any{&struct {
					UnderTest string `mapstructure:"test" yaml:"test"`
				}{UnderTest: "test"}},
			},
		},
		setupConfig: SetupConfig{
			FangsConfig: fangsConfig,
		},
	}
	err := showConfigInRootHelp(application)
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "error loading config object")
}

func Test_redactingHelpText(t *testing.T) {
	cfg := NewSetupConfig(Identification{
		Name:    "app",
		Version: "1.2.3",
	}).
		WithConfigInRootHelp().
		WithGlobalConfigFlag()

	r := &redactor{
		Value:  "asdf",
		redact: "asdf",
	}

	app := New(*cfg)

	root, _ := app.SetupRootCommand(&cobra.Command{}, r)

	r.store = app.(*application).state.RedactStore

	stdout, _ := captureStd(func() { _ = root.Help() })

	require.NotContains(t, stdout, "asdf")
	require.Contains(t, stdout, "value: '*******'")
}

func captureStd(fn func()) (stdout string, stderr string) {
	oldOut := os.Stdout // keep backup of the real stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	out := make(chan string)
	// copy the output in a separate goroutine so printing can't block indefinitely
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, outR)
		out <- buf.String()
	}()

	oldErr := os.Stderr // keep backup of the real stderr
	errR, errW, _ := os.Pipe()
	os.Stderr = errW

	err := make(chan string)
	// copy the output in a separate goroutine so printing can't block indefinitely
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, errR)
		err <- buf.String()
	}()

	fn()

	// back to normal state
	_ = outW.Close()
	os.Stdout = oldOut // restoring the real stdout

	_ = errW.Close()
	os.Stdout = oldErr // restoring the real stderr

	return <-out, <-err
}

type redactor struct {
	Value  string `mapstructure:"value"`
	redact string
	store  redact.Store
}

func (r *redactor) PostLoad() error {
	r.store.Add(r.redact)
	return nil
}
