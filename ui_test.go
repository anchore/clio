package clio

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/wagoodman/go-partybus"
)

type uiMocker struct {
	mock.Mock
}

func (m *uiMocker) Setup(subscription partybus.Unsubscribable) error {
	args := m.Called(subscription)
	return args.Error(0)
}

func (m *uiMocker) Handle(event partybus.Event) error {
	args := m.Called(event)
	return args.Error(0)
}

func (m *uiMocker) Teardown(force bool) error {
	args := m.Called(force)
	return args.Error(0)
}

type mockUnsubscribable struct {
	mock.Mock
}

func (m *mockUnsubscribable) Unsubscribe() error {
	args := m.Called()
	return args.Error(0)
}

func TestUICollection_Setup(t *testing.T) {
	subscription := &mockUnsubscribable{}
	ui1 := &uiMocker{}
	ui2 := &uiMocker{}

	ui1.On("Setup", subscription).Return(nil)

	c := NewUICollection(ui1, ui2)

	err := c.Setup(subscription)
	assert.NoError(t, err)
	assert.Equal(t, ui1, c.active)

	// first one wins!
	ui1.AssertCalled(t, "Setup", subscription)
}

func TestUICollection_Setup_FallbackOnError(t *testing.T) {
	subscription := &mockUnsubscribable{}
	ui1 := &uiMocker{}
	ui2 := &uiMocker{}

	ui1.On("Setup", subscription).Return(errors.New("setup failed")) // failure!
	ui2.On("Setup", subscription).Return(nil)                        // active ui...

	c := NewUICollection(ui1, ui2)

	err := c.Setup(subscription)
	assert.NoError(t, err)
	assert.Equal(t, ui2, c.active)
	// we attempted to call the first setup
	ui1.AssertCalled(t, "Setup", subscription)
	// we fallback to other uis if the first one fails
	ui2.AssertCalled(t, "Setup", subscription)
}

func TestUICollection_Handle(t *testing.T) {
	event := partybus.Event{}
	ui := &uiMocker{}

	ui.On("Setup", nil).Return(nil)
	ui.On("Handle", event).Return(nil)

	c := NewUICollection(ui)
	require.NoError(t, c.Setup(nil))

	err := c.Handle(event)
	assert.NoError(t, err)
	ui.AssertCalled(t, "Handle", event)
}

func TestUICollection_Handle_NoActiveUI(t *testing.T) {
	event := partybus.Event{}
	c := NewUICollection()

	err := c.Handle(event)
	assert.NoError(t, err, "Expected no error when no active UI is present")
}

func TestUICollection_Teardown(t *testing.T) {
	ui := &uiMocker{}
	ui.On("Setup", nil).Return(nil)
	ui.On("Teardown", false).Return(nil)

	c := NewUICollection(ui)
	require.NoError(t, c.Setup(nil))

	err := c.Teardown(false)
	assert.NoError(t, err)
	ui.AssertCalled(t, "Teardown", false)
}

func TestUICollection_Teardown_NoActiveUI(t *testing.T) {
	c := NewUICollection()

	err := c.Teardown(false)
	assert.NoError(t, err, "Expected no error when no active UI is present")
}

func TestUICollection_Replace(t *testing.T) {
	subscription := &mockUnsubscribable{}
	ui1 := &uiMocker{} // og, active
	ui2 := &uiMocker{} // og
	ui3 := &uiMocker{} // replacement

	ui1.On("Setup", subscription).Return(nil)
	ui2.On("Setup", subscription).Return(nil)
	ui3.On("Setup", subscription).Return(nil)
	ui1.On("Teardown", false).Return(nil)

	c := NewUICollection(ui1, ui2)
	require.NoError(t, c.Setup(subscription))

	err := c.Replace(ui3)
	assert.NoError(t, err)
	assert.Equal(t, ui3, c.active)
	ui1.AssertCalled(t, "Teardown", false)
	ui2.AssertNotCalled(t, "Teardown", subscription) // note, ui2 was never setup
	ui3.AssertCalled(t, "Setup", subscription)
}

func TestUICollection_Replace_ErrorDuringTeardown(t *testing.T) {
	subscription := &mockUnsubscribable{}
	ui1 := &uiMocker{}
	ui2 := &uiMocker{}
	ui3 := &uiMocker{}

	ui1.On("Setup", subscription).Return(nil)
	ui2.On("Setup", subscription).Return(nil)
	ui1.On("Teardown", false).Return(errors.New("teardown error"))

	uicollection := NewUICollection(ui1, ui2)
	require.NoError(t, uicollection.Setup(subscription))

	err := uicollection.Replace(ui3)
	require.Error(t, err)
	assert.EqualError(t, err, "unable to teardown existing UI: teardown error")
}

func TestUICollection_Replace_ErrorDuringSetup(t *testing.T) {
	subscription := &mockUnsubscribable{}
	ui1 := &uiMocker{}
	ui2 := &uiMocker{}
	ui3 := &uiMocker{}

	ui1.On("Setup", subscription).Return(nil)
	ui2.On("Setup", subscription).Return(nil)
	ui1.On("Teardown", false).Return(nil)
	ui3.On("Setup", subscription).Return(errors.New("setup error"))

	c := NewUICollection(ui1, ui2)
	require.NoError(t, c.Setup(subscription))

	err := c.Replace(ui3)
	require.Error(t, err)
	assert.EqualError(t, err, "unable to setup UI replacement: setup error")
}
