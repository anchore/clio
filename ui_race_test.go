package clio

import (
	"sync"
	"testing"

	"github.com/wagoodman/go-partybus"
)

type noopUI struct{}

func (noopUI) Setup(partybus.Unsubscribable) error { return nil }
func (noopUI) Handle(partybus.Event) error         { return nil }
func (noopUI) Teardown(bool) error                 { return nil }

// Handle reads uis, active and subscription, and Replace writes them under the
// lock. With a value receiver the whole collection is copied at the call site,
// before Lock() runs, so the read is unsynchronized.
func TestUICollectionHandleReplaceRace(t *testing.T) {
	c := NewUICollection(noopUI{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = c.Handle(partybus.Event{})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = c.Replace(noopUI{})
		}
	}()
	wg.Wait()
}
