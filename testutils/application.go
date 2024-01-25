package testutils

import (
	"github.com/anchore/clio"
	"github.com/spf13/cobra"
	"testing"
)

func NewForTesting(t *testing.T, cfg *clio.SetupConfig, assertion assertionFunc) clio.Application {
	a := clio.New(*cfg)

	//type Tester interface {
	//	WithTesting(assertion func(cfgs ...any)) clio.Application
	//}
	//
	//var asserter func(...any) = func(cfgs ...any) {
	//	assertion(t, cfgs)
	//}
	//
	//a = a.(Tester).WithTesting(asserter)

	return &testApplication{
		a,
		assertion,
		t,
	}
}

type assertionFunc func(t *testing.T, cfgs ...any)

type testApplication struct {
	clio.Application
	assertion assertionFunc
	t         *testing.T
}

func (a *testApplication) SetupCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		a.assertion(a.t, cfgs...)
		return nil
	}
	return a.Application.SetupCommand(cmd, cfgs...)
}

func (a *testApplication) SetupRootCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		a.assertion(a.t, cfgs...)
		return nil
	}
	return a.Application.SetupRootCommand(cmd, cfgs...)
}
