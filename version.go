package clio

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// Version defines the application version details (generally from build information)
type Version struct {
	Version        string `json:"version,omitempty"`        // application semantic version
	GitCommit      string `json:"gitCommit,omitempty"`      // git SHA at build-time
	GitDescription string `json:"gitDescription,omitempty"` // indication of git tree (either "clean" or "dirty") at build-time
	BuildDate      string `json:"buildDate,omitempty"`      // date of the build
}

type runtimeInfo struct {
	Application string `json:"application,omitempty"` // application name
	Version
	GoVersion string `json:"goVersion,omitempty"` // go runtime version at build-time
	Compiler  string `json:"compiler,omitempty"`  // compiler used at build-time
	Platform  string `json:"platform,omitempty"`  // GOOS and GOARCH at build-time
}

func VersionCommand(a Application, version Version) *cobra.Command {
	var format string

	info := runtimeInfo{
		Application: a.Config().Name,
		Version:     version,
		GoVersion:   runtime.Version(),
		Compiler:    runtime.Compiler,
		Platform:    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "show version information",
		Args:  cobra.NoArgs,
		// note: we intentionally do not execute through the application infrastructure (no app config is required for this command)
		RunE: func(cmd *cobra.Command, args []string) error {
			switch format {
			case "text", "":
				printIfNotEmpty("Application", info.Application)
				printIfNotEmpty("Version", info.Version.Version)
				printIfNotEmpty("BuildDate", info.BuildDate)
				printIfNotEmpty("GitCommit", info.GitCommit)
				printIfNotEmpty("GitDescription", info.GitDescription)
				printIfNotEmpty("Platform", info.Platform)
				printIfNotEmpty("GoVersion", info.GoVersion)
				printIfNotEmpty("Compiler", info.Compiler)

			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetEscapeHTML(false)
				enc.SetIndent("", " ")
				err := enc.Encode(info)
				if err != nil {
					return fmt.Errorf("failed to show version information: %w", err)
				}
			default:
				return fmt.Errorf("unsupported output format: %s", format)
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&format, "output", "o", "text", "the format to show the results (allowable: [text json])")

	return cmd
}

func printIfNotEmpty(title, value string) {
	if value == "" {
		return
	}

	fmt.Printf("%-16s %s\n", title+":", value)
}
