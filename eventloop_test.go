package clio

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/wagoodman/go-partybus"

	"github.com/anchore/go-logger/adapter/discard"
)

const exitEvent partybus.EventType = "testing-exit"

var _ UI = (*uiMock)(nil)

type uiMock struct {
	t            *testing.T
	finalEvent   partybus.Event
	subscription partybus.Unsubscribable
	mock.Mock
}

func (u *uiMock) Setup(subscription partybus.Unsubscribable) error {
	u.t.Logf("UI Setup called")
	u.subscription = subscription
	return u.Called(subscription.Unsubscribe).Error(0)
}

func (u *uiMock) Handle(event partybus.Event) error {
	u.t.Logf("UI Handle called: %+v", event.Type)
	if event == u.finalEvent {
		assert.NoError(u.t, u.subscription.Unsubscribe())
	}
	return u.Called(event).Error(0)
}

func (u *uiMock) Teardown(force bool) error {
	u.t.Logf("UI Teardown called")
	return u.Called(force).Error(0)
}

func Test_EventLoop_gracefulExit(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		finalEvent := partybus.Event{
			Type: exitEvent,
		}

		worker := func() <-chan error {
			ret := make(chan error)
			go func() {
				t.Log("worker running")
				// send an empty item (which is ignored) ensuring we've entered the select statement,
				// then close (a partial shutdown).
				ret <- nil
				t.Log("worker sent nothing")
				close(ret)
				t.Log("worker closed")
				// do the other half of the shutdown
				testBus.Publish(finalEvent)
				t.Log("worker published final event")
			}()
			return ret
		}

		ux := &uiMock{
			t:          t,
			finalEvent: finalEvent,
		}

		// ensure the mock sees at least the final event
		ux.On("Handle", finalEvent).Return(nil)
		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", false).Return(nil)

		assert.NoError(t,
			eventloop(
				context.Background(),
				discard.New(),
				subscription,
				worker(),
				ux,
			),
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func Test_EventLoop_workerError(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		workerErr := fmt.Errorf("worker error")

		worker := func() <-chan error {
			ret := make(chan error)
			go func() {
				t.Log("worker running")
				// send an empty item (which is ignored) ensuring we've entered the select statement,
				// then close (a partial shutdown).
				ret <- nil
				t.Log("worker sent nothing")
				ret <- workerErr
				t.Log("worker sent error")
				close(ret)
				t.Log("worker closed")
				// note: NO final event is fired
			}()
			return ret
		}

		ux := &uiMock{
			t: t,
		}

		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", true).Return(nil)

		// ensure we see an error returned
		assert.ErrorIs(t,
			eventloop(
				context.Background(),
				discard.New(),
				subscription,
				worker(),
				ux,
			),
			workerErr,
			"should have seen a worker error, but did not",
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func Test_EventLoop_unsubscribeError(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		finalEvent := partybus.Event{
			Type: exitEvent,
		}

		worker := func() <-chan error {
			ret := make(chan error)
			go func() {
				t.Log("worker running")
				// send an empty item (which is ignored) ensuring we've entered the select statement,
				// then close (a partial shutdown).
				ret <- nil
				t.Log("worker sent nothing")
				close(ret)
				t.Log("worker closed")
				// do the other half of the shutdown
				testBus.Publish(finalEvent)
				t.Log("worker published final event")
			}()
			return ret
		}

		ux := &uiMock{
			t:          t,
			finalEvent: finalEvent,
		}

		// ensure the mock sees at least the final event... note the unsubscribe error here
		ux.On("Handle", finalEvent).Return(partybus.ErrUnsubscribe)
		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", false).Return(nil)

		// unsubscribe errors should be handled and ignored, not propagated. We are additionally asserting that
		// this case is handled as a controlled shutdown (this test should not timeout)
		assert.NoError(t,
			eventloop(
				context.Background(),
				discard.New(),
				subscription,
				worker(),
				ux,
			),
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func Test_EventLoop_handlerError(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		finalEvent := partybus.Event{
			Type:  exitEvent,
			Error: fmt.Errorf("an exit error occured"),
		}

		worker := func() <-chan error {
			ret := make(chan error)
			go func() {
				t.Log("worker running")
				// send an empty item (which is ignored) ensuring we've entered the select statement,
				// then close (a partial shutdown).
				ret <- nil
				t.Log("worker sent nothing")
				close(ret)
				t.Log("worker closed")
				// do the other half of the shutdown
				testBus.Publish(finalEvent)
				t.Log("worker published final event")
			}()
			return ret
		}

		ux := &uiMock{
			t:          t,
			finalEvent: finalEvent,
		}

		// ensure the mock sees at least the final event... note the event error is propagated
		ux.On("Handle", finalEvent).Return(finalEvent.Error)
		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", false).Return(nil)

		// handle errors SHOULD propagate the event loop. We are additionally asserting that this case is
		// handled as a controlled shutdown (this test should not timeout)
		assert.ErrorIs(t,
			eventloop(
				context.Background(),
				discard.New(),
				subscription,
				worker(),
				ux,
			),
			finalEvent.Error,
			"should have seen a event error, but did not",
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func Test_EventLoop_contextCancelStopExecution(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		worker := func() <-chan error {
			// the worker will never return work and the event loop will always be waiting...
			return make(chan error)
		}

		ctx, cancel := context.WithCancel(context.Background())

		ux := &uiMock{
			t: t,
		}

		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", true).Return(nil)

		go cancel()

		assert.NoError(t,
			eventloop(
				ctx,
				discard.New(),
				subscription,
				worker(),
				ux,
			),
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func Test_EventLoop_ExitEventStopExecution(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		finalEvent := ExitEvent(false)

		worker := func() <-chan error {
			ret := make(chan error)
			go func() {
				t.Log("worker running")
				// send an empty item (which is ignored) ensuring we've entered the select statement,
				// then close (a partial shutdown).
				ret <- nil
				t.Log("worker sent nothing")
				close(ret)
				t.Log("worker closed")
				// do the other half of the shutdown
				testBus.Publish(finalEvent)
				t.Log("worker published final event")
			}()
			return ret
		}

		ux := &uiMock{
			t: t,
			// don't force unsubscribe, allow exit to cause it
		}

		// ensure the mock sees at least the final event... note the event error is propagated
		ux.On("Handle", finalEvent).Return(nil)
		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", false).Return(nil)

		// handle errors SHOULD propagate the event loop. We are additionally asserting that this case is
		// handled as a controlled shutdown (this test should not timeout)
		assert.ErrorIs(t,
			eventloop(
				context.Background(),
				discard.New(),
				subscription,
				worker(),
				ux,
			),
			finalEvent.Error,
			"should have seen a event error, but did not",
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func Test_EventLoop_InterruptEventKillExecution(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		finalEvent := ExitEvent(true)

		worker := func() <-chan error {
			ret := make(chan error)
			go func() {
				t.Log("worker running")
				// send an empty item (which is ignored) ensuring we've entered the select statement,
				// then close (a partial shutdown).
				ret <- nil
				t.Log("worker sent nothing")
				close(ret)
				t.Log("worker closed")
				// do the other half of the shutdown
				testBus.Publish(finalEvent)
				t.Log("worker published final event")
			}()
			return ret
		}

		ux := &uiMock{
			t: t,
			// don't force unsubscribe, allow exit to cause it
		}

		// ensure the mock sees at least the final event... note the event error is propagated
		ux.On("Handle", finalEvent).Return(nil)
		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", true).Return(nil)

		// handle errors SHOULD propagate the event loop. We are additionally asserting that this case is
		// handled as a controlled shutdown (this test should not timeout)
		assert.ErrorIs(t,
			eventloop(
				context.Background(),
				discard.New(),
				subscription,
				worker(),
				ux,
			),
			finalEvent.Error,
			"should have seen a event error, but did not",
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func Test_EventLoop_uiTeardownError(t *testing.T) {
	test := func(t *testing.T) {

		testBus := partybus.NewBus()
		subscription := testBus.Subscribe()
		t.Cleanup(testBus.Close)

		finalEvent := partybus.Event{
			Type: exitEvent,
		}

		worker := func() <-chan error {
			ret := make(chan error)
			go func() {
				t.Log("worker running")
				// send an empty item (which is ignored) ensuring we've entered the select statement,
				// then close (a partial shutdown).
				ret <- nil
				t.Log("worker sent nothing")
				close(ret)
				t.Log("worker closed")
				// do the other half of the shutdown
				testBus.Publish(finalEvent)
				t.Log("worker published final event")
			}()
			return ret
		}

		ux := &uiMock{
			t:          t,
			finalEvent: finalEvent,
		}

		teardownError := fmt.Errorf("sorry, dave, the UI doesn't want to be torn down")

		// ensure the mock sees at least the final event... note the event error is propagated
		ux.On("Handle", finalEvent).Return(nil)
		// ensure the mock sees basic setup/teardown events
		ux.On("Setup", mock.AnythingOfType("func() error")).Return(nil)
		ux.On("Teardown", false).Return(teardownError)

		// ensure we see an error returned
		assert.ErrorIs(t,
			eventloop(
				context.Background(),
				discard.New(),
				subscription,
				worker(),
				ux,
			),
			teardownError,
			"should have seen a UI teardown error, but did not",
		)

		ux.AssertExpectations(t)
	}

	// if there is a bug, then there is a risk of the event loop never returning
	testWithTimeout(t, 5*time.Second, test)
}

func testWithTimeout(t *testing.T, timeout time.Duration, test func(*testing.T)) {
	done := make(chan bool)
	go func() {
		test(t)
		done <- true
	}()

	select {
	case <-time.After(timeout):
		t.Fatal("test timed out")
	case <-done:
	}
}
