package clio

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"

	"github.com/anchore/fangs"
)

func ConfigCommand(app Application, options ...configCommandOption) *cobra.Command {
	opts := configCommandOptions{}
	for _, option := range options {
		option(&opts)
	}

	id := app.ID()
	internalApp := extractInternalApp(app)
	if internalApp == nil {
		return &cobra.Command{
			RunE: func(_ *cobra.Command, _ []string) error {
				return fmt.Errorf("unable to extract internal application, provided: %v", app)
			},
		}
	}

	cmd := &cobra.Command{
		Use:   "config",
		Short: fmt.Sprintf("show the %s configuration", id.Name),
		RunE: func(cmd *cobra.Command, _ []string) error {
			allConfigs := allCommandConfigs(internalApp)
			var err error
			if opts.loadConfig {
				err = loadAllConfigs(cmd, internalApp.setupConfig.FangsConfig, allConfigs)
			}
			filter := opts.valueFilterFunc
			if internalApp.state.RedactStore != nil {
				filter = chainFilterFuncs(internalApp.state.RedactStore.RedactString, filter)
			}
			summary := summarizeConfig(cmd, internalApp.setupConfig.FangsConfig, filter, allConfigs)
			_, writeErr := os.Stdout.WriteString(summary)
			if writeErr != nil {
				writeErr = fmt.Errorf("an error occurred writing configuration summary: %w", writeErr)
				err = errors.Join(err, writeErr)
			}
			if err != nil {
				// space before the error display
				_, _ = os.Stderr.WriteString("\n")
			}
			return err
		},
	}

	cmd.Flags().BoolVarP(&opts.loadConfig, "load", "", opts.loadConfig, fmt.Sprintf("load and validate the %s configuration", id.Name))

	if opts.includeLocationsSubcommand {
		// sub-command to print expanded configuration file search locations
		cmd.AddCommand(summarizeLocationsCommand(internalApp))
	}

	return cmd
}

type configCommandOption func(*configCommandOptions)

type valueFilterFunc func(string) string

type configCommandOptions struct {
	loadConfig                 bool
	includeLocationsSubcommand bool
	valueFilterFunc            valueFilterFunc
}

// ReplaceHomeDirWithTilde adds a value filter function which replaces matching home directory values in strings
// starting with the user's home directory to make configurations more portable
func ReplaceHomeDirWithTilde(opts *configCommandOptions) {
	userHome, _ := homedir.Dir()
	if userHome != "" {
		opts.valueFilterFunc = chainFilterFuncs(opts.valueFilterFunc, func(s string) string {
			// make any defaults based on the user's home directory more portable
			if strings.HasPrefix(s, userHome) {
				s = strings.ReplaceAll(s, userHome, "~")
			}
			return s
		})
	}
}

// IncludeLocationsSubcommand will include a `config locations` subcommand which lists each location that will be used
// to locate configuration files based on the configured environment
func IncludeLocationsSubcommand(opts *configCommandOptions) {
	opts.includeLocationsSubcommand = true
}

func chainFilterFuncs(f1, f2 valueFilterFunc) valueFilterFunc {
	if f1 == nil {
		return f2
	}
	if f2 == nil {
		return f1
	}
	return func(s string) string {
		s = f1(s)
		s = f2(s)
		return s
	}
}

func extractInternalApp(app Application) *application {
	if a, ok := app.(*application); ok {
		return a
	}
	return nil
}

func allCommandConfigs(internalApp *application) []any {
	return append([]any{&internalApp.state.Config, internalApp}, internalApp.state.Config.FromCommands...)
}

func loadAllConfigs(cmd *cobra.Command, fangsCfg fangs.Config, allConfigs []any) error {
	var errs []error
	for _, cfg := range allConfigs {
		// load each config individually, as there may be conflicting names / types that will cause
		// viper to fail to read them all and panic
		if err := fangs.Load(fangsCfg, cmd, cfg); err != nil {
			t := reflect.TypeOf(cfg)
			for t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			errs = append(errs, fmt.Errorf("error loading config %s: %w", t.Name(), err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("error(s) occurred loading configuration: %w", errors.Join(errs...))
}

func summarizeConfig(commandWithRootParent *cobra.Command, fangsCfg fangs.Config, redact func(string) string, allConfigs []any) string {
	summary := fangs.SummarizeCommand(fangsCfg, commandWithRootParent, redact, allConfigs...)
	summary = strings.TrimSpace(summary) + "\n"
	return summary
}

func summarizeLocationsCommand(internalApp *application) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "locations",
		Short: fmt.Sprintf("shows all locations and the order in which %s will look for a configuration file", internalApp.ID().Name),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			suffix := ".yaml"
			if all {
				suffix = ""
			}
			summary := summarizeLocations(internalApp.setupConfig.FangsConfig, suffix)
			_, err := os.Stdout.WriteString(summary)
			return err
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "", all, "include every file extension supported")

	return cmd
}

func summarizeLocations(fangsCfg fangs.Config, onlySuffix string) string {
	out := ""
	for _, f := range fangs.SummarizeLocations(fangsCfg) {
		if onlySuffix != "" && !strings.HasSuffix(f, onlySuffix) {
			continue
		}
		out += f + "\n"
	}
	return out
}
