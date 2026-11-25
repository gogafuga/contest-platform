package analyzer

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCapacityCoversClassWithoutChangingContainerBudget(t *testing.T) {
	m := testManager(6)
	capacity := m.Capacity()
	if capacity.MaxSessions != 6 || capacity.RequestSlots != 44 || capacity.AdmissionBlocked {
		t.Fatalf("unexpected capacity: %+v", capacity)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errors := make(chan error, 25)
	for owner := int64(1); owner <= 25; owner++ {
		wg.Add(1)
		go func(owner int64) {
			defer wg.Done()
			for {
				e, created, err := m.acquire(ctx, testRequest(owner))
				if err == errQueued {
					continue
				}
				if err != nil {
					errors <- err
					return
				}
				if !created {
					t.Error("different owners shared an entry")
					return
				}
				m.mu.Lock()
				if m.live > 6 {
					t.Error("container budget exceeded")
				}
				e.created = time.Now().Add(-pressureLease - time.Second)
				close(e.ready)
				m.mu.Unlock()
				m.finished(e)
				return
			}
		}(owner)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	m.Close()
	if capacity = m.Capacity(); capacity.LiveSessions != 0 || !capacity.AdmissionBlocked {
		t.Fatalf("shutdown capacity: %+v", capacity)
	}
}
