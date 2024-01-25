package clio

import (
	"github.com/anchore/fangs"
)

// we can put this on application struct without needing to expose it in the interface (just provide the func assertion)
func NewForTesting(assertion func(cfgs ...any)) Application {
	a := New(SetupConfig{
		ID: Identification{
			Name:           "testing",
			Version:        "string",
			GitCommit:      "",
			GitDescription: "",
			BuildDate:      "",
		},
		DefaultLoggingConfig:     nil,
		DefaultDevelopmentConfig: nil,
		FangsConfig:              fangs.Config{},
		BusConstructor:           nil,
		LoggerConstructor:        nil,
		UIConstructor:            nil,
		Initializers:             nil,
	})

	b := a.(*application)
	b.testing = assertion
	return b
}
