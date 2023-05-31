package clio

import (
	"context"
	"errors"

	"github.com/hashicorp/go-multierror"
	"github.com/wagoodman/go-partybus"

	"github.com/anchore/go-logger"
)

// eventloop listens to worker errors (from execution path), worker events (from a partybus subscription), and
// signal interrupts. Is responsible for handling each event relative to a given UI to coordinate eventing until
// an eventual graceful exit.
//
//nolint:gocognit,funlen
func eventloop(ctx context.Context, log logger.Logger, subscription *partybus.Subscription, workerErrs <-chan error, uis ...UI) error {
	var events <-chan partybus.Event
	if subscription != nil {
		events = subscription.Events()
	} else {
		noEvents := make(chan partybus.Event)
		close(noEvents)
		events = noEvents
	}

	var ux UI

	for _, ui := range uis {
		if err := ui.Setup(subscription); err != nil {
			log.Warnf("unable to setup given UI, falling back to alternative UI: %+v", err)
			continue
		}

		ux = ui
		break
	}

	var retErr error
	var forceTeardown bool

	for {
		if workerErrs == nil && events == nil {
			break
		}
		select {
		case err, isOpen := <-workerErrs:
			if !isOpen {
				log.Trace("worker stopped")
				workerErrs = nil
				continue
			}
			if err != nil {
				// capture the error from the worker and unsubscribe to complete a graceful shutdown
				retErr = multierror.Append(retErr, err)
				if subscription != nil {
					_ = subscription.Unsubscribe()
				}
				// the worker has exited, we may have been mid-handling events for the UI which should now be
				// ignored, in which case forcing a teardown of the UI regardless of the state is required.
				forceTeardown = true
			}
		case e, isOpen := <-events:
			if !isOpen {
				log.Trace("bus stopped")
				events = nil
				continue
			}
			if ux == nil {
				continue
			}
			if err := ux.Handle(e); err != nil {
				if errors.Is(err, partybus.ErrUnsubscribe) {
					events = nil
				} else {
					retErr = multierror.Append(retErr, err)
					// TODO: should we unsubscribe? should we try to halt execution? or continue?
				}
			}
		case <-ctx.Done():
			log.Trace("signal interrupt")

			// ignore further results from any event source and exit ASAP, but ensure that all cache is cleaned up.
			// we ignore further errors since cleaning up the tmp directories will affect running catalogers that are
			// reading/writing from/to their nested temp dirs. This is acceptable since we are bailing without result.

			// TODO: potential future improvement would be to pass context into workers with a cancel function that is
			// to the event loop. In this way we can have a more controlled shutdown even at the most nested levels
			// of processing.
			events = nil
			workerErrs = nil
			forceTeardown = true
		}
	}
	if ux != nil {
		if err := ux.Teardown(forceTeardown); err != nil {
			retErr = multierror.Append(retErr, err)
		}
	}

	return retErr
}
