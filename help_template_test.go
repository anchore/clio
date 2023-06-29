package clio

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/anchore/go-logger/adapter/redact"
)

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

	root := &cobra.Command{}
	app := New(*cfg, root, r)

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
