package cliotestutils

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/anchore/clio"
)

// NewApplication takes a testing.T, a clio setup config, and a slice of assertions, and returns
// a clio application that will, instead of setting up commands with their normal RunE, set up commands
// such that the assertions are called with the testing.T after config state is set up by reading flags,
// env vars, and config files. Useful for testing that expected configuration options are wired up.
// Note that initializers will be cleared from the clio setup config, since the initialization may happen
// more than once and affect global state. For necessary global state, a workaround is to set it in a TestingMain.
func NewApplication(t *testing.T, cfg *clio.SetupConfig, assertions ...AssertionFunc) clio.Application {
	cfg.Initializers = nil
	a := clio.New(*cfg)

	var asserter assertionClosure = func(cmd *cobra.Command, args []string, cfgs ...any) {
		for _, assertion := range assertions {
			assertion(t, cmd, args, cfgs...)
		}
	}

	return &testApplication{
		a,
		asserter,
	}
}

type AssertionFunc func(t *testing.T, cmd *cobra.Command, args []string, cfgs ...any)

func OptionsEquals(wantOpts any) AssertionFunc {
	return func(t *testing.T, cmd *cobra.Command, args []string, cfgs ...any) {
		assert.Equal(t, len(cfgs), 1)
		if d := cmp.Diff(wantOpts, cfgs[0]); d != "" {
			t.Errorf("mismatched options (-want +got):\n%s", d)
		}
	}
}

type assertionClosure func(cmd *cobra.Command, args []string, cfgs ...any)

type testApplication struct {
	clio.Application
	assertion assertionClosure
}

func (a *testApplication) SetupCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		a.assertion(cmd, args, cfgs...)
		return nil
	}
	return a.Application.SetupCommand(cmd, cfgs...)
}

func (a *testApplication) SetupRootCommand(cmd *cobra.Command, cfgs ...any) *cobra.Command {
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		a.assertion(cmd, args, cfgs...)
		return nil
	}
	return a.Application.SetupRootCommand(cmd, cfgs...)
}

/*
// TODO: WISHLIST:
1. Helper to wire up a test fixture as the only config file that will be found
2. Set env vars by passing map[string]string (currently possible by caller in test; a helper here would be nice.)
*/
