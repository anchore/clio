package testutils

import (
	"github.com/anchore/clio"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

// TODO: is this needed given WrapForTesting? We need to think about
// how clio is handling global state.
// in particular, the testing pattern can cause initializers to be called more than once.
func NewForTesting(t *testing.T, cfg *clio.SetupConfig, assertions ...AssertionFunc) clio.Application {
	a := clio.New(*cfg)

	var asserter assertionClosure = func(cmd *cobra.Command, args []string, cfgs ...any) {
		for _, assertion := range assertions {
			assertion(t, cmd, args, cfgs...)
		}
	}

	return &testApplication{
		a,
		asserter,
		sync.Once{},
	}
}

func WrapForTesting(t *testing.T, a clio.Application, assertions ...AssertionFunc) clio.Application {
	var asserter assertionClosure = func(cmd *cobra.Command, args []string, cfgs ...any) {
		for _, assertion := range assertions {
			assertion(t, cmd, args, cfgs...)
		}
	}

	return &testApplication{
		a,
		asserter,
		sync.Once{},
	}
}

type AssertionFunc func(t *testing.T, cmd *cobra.Command, args []string, cfgs ...any)

func OptionsEquals(opts any) AssertionFunc {
	return func(t *testing.T, cmd *cobra.Command, args []string, cfgs ...any) {
		assert.Equal(t, len(cfgs), 1)
		if d := cmp.Diff(opts, cfgs[0]); d != "" {
			t.Errorf("mismatched options (+got -want):\n%s", d)
		}
	}
}

type assertionClosure func(cmd *cobra.Command, args []string, cfgs ...any)

type testApplication struct {
	clio.Application
	assertion assertionClosure
	initOnce  sync.Once
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
1. Pass in an io.Reader that's wired up to the config file, _or_ (almost as good) pass in a test fixture path
2. Set env vars by passing map[string]string
3. Helper to pass in a fully populated options struct and assert that it's correct
*/
