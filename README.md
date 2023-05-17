# CLIO

An easy way to bootstrap your application with batteries included.

## What is included?
- Pairs well with [cobra](github.com/spf13/cobra) and [viper](github.com/spf13/viper) via [fangs](github.com/anchore/fangs), covering CLI arg parsing and config file + env var loading.
- Provides an event bus via [partybus](github.com/wagoodman/go-partybus), enabling visibility deep in your execution stack as to what is happening.
- Provides a logger via the [logger interface](github.com/anchore/go-logger), allowing you to swap out for any concrete logger you want and decorate with redaction capabilities.
- Defines a generic UI interface that adapts well to TUI frameworks such as [bubbletea](github.com/charmbracelet/bubbletea).

## Example

Here's a basic example of how to use clio + cobra to get a fully functional CLI application going:

```go
package main

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/wagoodman/go-partybus"
	"github.com/anchore/clio"
)

// Define your per-command or entire application config as a struct
type MyCommandConfig struct {
	TimestampServer string `yaml:"timestamp-server" mapstructure:"timestamp-server"`
	// ...
}

// ... add cobra flags just as you are used to doing in any other cobra application
func (c *MyCommandConfig) AddFlags(flags *pflag.FlagSet) {
	flags.StringVarP(
		&c.TimestampServer, "timestamp-server", "", c.TimestampServer,
		"URL to a timestamp server to use for timestamping the signature",
	)
	// ...
}

func MyCommand(app clio.Application) *cobra.Command {
	cfg := &MyCommandConfig{
		TimestampServer: "https://somewhere.out/there", // a default value
	}

	return &cobra.Command{
		Use:     "my-command",
		PreRunE: app.Setup(cfg),
		RunE: func(cmd *cobra.Command, args []string) error {
			state := app.State()
			var log = state.Logger
			var bus partybus.Publisher = state.Bus

			log.Infof("hi! pinging the timestamp server: %s", cfg.TimestampServer)

			type myRichObject struct {
				data io.Reader
				// status progress.Progressable
			}

			bus.Publish(partybus.Event{
				Type: "something-notable",
				//Value:  myRichObject{...},
			})
			return nil
		},
	}
}

func main() {
	cfg := clio.NewConfig("awesome", "v1.0.0")
	app := clio.New(*cfg)

	cmd := MyCommand(app)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```