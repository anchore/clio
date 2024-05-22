package clio

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_versionOutputToStdout(t *testing.T) {
	c := VersionCommand(Identification{
		Name:    "test",
		Version: "version",
	})

	stdout, stderr := captureStd(func() {
		_ = c.RunE(c, nil)
	})

	require.NotEmpty(t, stdout)
	require.Empty(t, stderr)
}

func Test_versionInfoText(t *testing.T) {
	expected := `Application:               the-name
Version:                   the-version
BuildDate:                 the-build-date
GitCommit:                 the-commit
GitDescription:            the-description
Platform:                  linux/amd64
GoVersion:                 go1.21.1
Compiler:                  gc
Addition With A Long Line: some-value
`
	got, err := versionInfo(runtimeInfo{
		Identification: Identification{
			Name:           "the-name",
			Version:        "the-version",
			GitCommit:      "the-commit",
			GitDescription: "the-description",
			BuildDate:      "the-build-date",
		},
		GoVersion: "go1.21.1",
		Compiler:  "gc",
		Platform:  "linux/amd64",
	}, "text", func() (name string, value any) {
		return "Addition With A Long Line", "some-value"
	})
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func Test_versionInfoJSON(t *testing.T) {
	expected := `{
		"additionalValue": "some-value",
		"someValueWithSpacesAndUpper": "some-other-value",
		"application": "the-name",
		"buildDate": "the-build-date",
		"compiler": "gc",
		"gitCommit": "the-commit",
		"gitDescription": "the-description",
		"goVersion": "go1.21.1",
		"platform": "linux/amd64",
		"version": "the-version"
	}`
	got, err := versionInfo(runtimeInfo{
		Identification: Identification{
			Name:           "the-name",
			Version:        "the-version",
			GitCommit:      "the-commit",
			GitDescription: "the-description",
			BuildDate:      "the-build-date",
		},
		GoVersion: "go1.21.1",
		Compiler:  "gc",
		Platform:  "linux/amd64",
	}, "json", func() (name string, value any) {
		return "additionalValue", "some-value"
	}, func() (name string, value any) {
		return "Some Value With Spaces and UPPER", "some-other-value"
	})
	require.NoError(t, err)
	require.JSONEq(t, expected, got)
}

func captureStd(fn func()) (stdout, stderr string) {
	out := &bytes.Buffer{}
	restoreStdout := capture(&os.Stdout, out)
	defer restoreStdout()

	err := &bytes.Buffer{}
	restoreStderr := capture(&os.Stderr, err)
	defer restoreStderr()

	fn()

	// close and flush the buffers, wait until complete
	restoreStdout()
	restoreStderr()

	return out.String(), err.String()
}

func capture(target **os.File, writer io.Writer) (close func()) {
	original := *target

	r, w, _ := os.Pipe()

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				_, _ = writer.Write(buf[0:n])
			}
			if err != nil {
				break
			}
		}
	}()

	*target = w

	return func() {
		if original != nil {
			_ = w.Close()
			wg.Wait()
			*target = original
			original = nil
		}
	}
}
