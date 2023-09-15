package clio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
	}, "text", func(_ string) (name string, value any) {
		return "Addition With A Long Line", "some-value"
	})
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func Test_versionInfoJSON(t *testing.T) {
	expected := `{
		"additionalValue": "some-value",
		"someValueWithSpaces": "some-other-value",
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
	}, "json", func(_ string) (name string, value any) {
		return "additionalValue", "some-value"
	}, func(_ string) (name string, value any) {
		return "Some Value With Spaces", "some-other-value"
	})
	require.NoError(t, err)
	require.JSONEq(t, expected, got)
}
