package testutils

import (
	"github.com/anchore/clio"
	"github.com/anchore/fangs"
)

func NewForTesting(assertion func(cfgs ...any)) clio.Application {
	a := clio.New(clio.SetupConfig{
		ID: clio.Identification{
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

	type Tester interface {
		WithTesting(assertion func(cfgs ...any)) clio.Application
	}

	a = a.(Tester).WithTesting(assertion)

	return a
}
