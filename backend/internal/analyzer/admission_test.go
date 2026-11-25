package analyzer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func testManager(max int) *Manager {
	return &Manager{ctx: context.Background(), MaxSessions: max, sessions: make(map[string]*entry), changed: make(chan struct{})}
}
func testRequest(owner int64) AssistRequest {
	return AssistRequest{Owner: owner, Task: "test", ClientID: fmt.Sprintf("%032x", owner), Language: "go"}
}
func addEntry(m *Manager, r AssistRequest, active bool) *entry {
	e := &entry{ready: make(chan struct{}), owner: r.Owner, created: time.Now().Add(-time.Minute), lastUsed: time.Now().Add(-time.Minute)}
	if active {
		e.users = 1
	}
	close(e.ready)
	m.sessions[key(r)] = e
	m.live++
	return e
}
func waitQueue(t *testing.T, m *Manager, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.waiters)
		m.mu.Unlock()
		if n == count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("queue did not reach expected size")
}

func TestAdmissionReclaimsOnlyInactiveSessions(t *testing.T) {
	m := testManager(1)
	r := testRequest(1)
	old := addEntry(m, r, true)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, _, err := m.acquire(ctx, testRequest(2)); err != ErrBusy {
		t.Fatalf("active request was not protected: %v", err)
	}
	if m.sessions[key(r)] != old || m.live != 1 || len(m.waiters) != 0 {
		t.Fatal("active session was retired or canceled ticket leaked")
	}
	m.finished(old)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	e, created, err := m.acquire(ctx2, testRequest(2))
	if err != nil || !created || m.live != 1 {
		t.Fatalf("inactive lease was not reclaimed: %v", err)
	}
	close(e.ready)
	m.finished(e)
	m.Close()
}

func TestAdmissionFIFOAndCleanupReservation(t *testing.T) {
	m := testManager(1)
	// A retiring container has left the session map, but still consumes capacity.
	held := &entry{}
	m.live = 1
	type acquired struct {
		e   *entry
		err error
	}
	first := make(chan acquired, 1)
	second := make(chan acquired, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { e, _, err := m.acquire(ctx, testRequest(1)); first <- acquired{e, err} }()
	waitQueue(t, m, 1)
	go func() { e, _, err := m.acquire(ctx, testRequest(2)); second <- acquired{e, err} }()
	waitQueue(t, m, 2)
	select {
	case <-first:
		t.Fatal("capacity released before container cleanup")
	default:
	}
	m.release(held)
	a := <-first
	if a.err != nil {
		t.Fatal(a.err)
	}
	close(a.e.ready)
	select {
	case <-second:
		t.Fatal("FIFO follower overtook occupied capacity")
	default:
	}
	m.finished(a.e)
	m.removeIf(key(testRequest(1)), a.e)
	b := <-second
	if b.err != nil {
		t.Fatal(b.err)
	}
	close(b.e.ready)
	m.finished(b.e)
	m.Close()
}

func TestAdmissionOwnerLimitAndStaleRemoval(t *testing.T) {
	m := testManager(3)
	r1 := testRequest(1)
	r2 := r1
	r2.ClientID = "22222222222222222222222222222222"
	old := addEntry(m, r1, false)
	addEntry(m, r2, false)
	other := testRequest(2)
	otherEntry := addEntry(m, other, false)
	r3 := r1
	r3.ClientID = "33333333333333333333333333333333"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e, _, err := m.acquire(ctx, r3)
	if err != nil {
		t.Fatal(err)
	}
	close(e.ready)
	if m.sessions[key(other)] != otherEntry {
		t.Fatal("owner cap evicted somebody else's session")
	}
	m.removeIf(key(r3), old)
	if m.sessions[key(r3)] != e {
		t.Fatal("stale removal deleted replacement")
	}
	m.finished(e)
	m.Close()
}

func TestAdmissionQueueBoundAndClose(t *testing.T) {
	m := testManager(1)
	addEntry(m, testRequest(1), true)
	for i := 0; i < maxWaiting; i++ {
		m.waiters = append(m.waiters, &admission{key: fmt.Sprint(i), owner: int64(i + 100), queuedAt: time.Now(), lastSeen: time.Now(), retained: true})
	}
	if _, _, err := m.acquire(context.Background(), testRequest(2)); err != ErrBusy {
		t.Fatal("unbounded waiting queue")
	}
	m.waiters = nil
	result := make(chan error, 1)
	go func() { _, _, err := m.acquire(context.Background(), testRequest(2)); result <- err }()
	waitQueue(t, m, 1)
	m.Close()
	if err := <-result; err != ErrUnavailable {
		t.Fatalf("shutdown did not wake waiter: %v", err)
	}
}

func TestAdmissionPollRetainsFIFOAndCloseCancels(t *testing.T) {
	m := testManager(1)
	old := addEntry(m, testRequest(1), true)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, _, err := m.acquire(ctx, testRequest(2)); err != errQueued {
		t.Fatalf("want short pending response: %v", err)
	}
	if len(m.waiters) != 1 || m.waiters[0].requests != 0 || !m.waiters[0].retained {
		t.Fatal("pending ticket not retained")
	}
	ticket := m.waiters[0]
	canceledPoll, cancelPoll := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if _, _, err := m.acquire(canceledPoll, testRequest(2)); err != ErrBusy {
		t.Fatal(err)
	}
	cancelPoll()
	if len(m.waiters) != 1 || m.waiters[0] != ticket {
		t.Fatal("canceled later poll lost retained place")
	}
	if _, _, err := m.acquire(ctx, testRequest(2)); err != errQueued {
		t.Fatal(err)
	}
	if len(m.waiters) != 1 || m.waiters[0] != ticket {
		t.Fatal("poll changed FIFO position")
	}
	m.finished(old)
	m.removeIf(key(testRequest(1)), old)
	e, created, err := m.acquire(ctx, testRequest(2))
	if err != nil || !created {
		t.Fatal(err)
	}
	if e.admissionWait < 2*admissionPoll {
		t.Fatal("wait timing lost earlier polls")
	}
	close(e.ready)
	m.finished(e)
	m.removeIf(key(testRequest(2)), nil)
	m.Close()
}

func TestManagerQueuedResponseAndExplicitCancel(t *testing.T) {
	m := testManager(1)
	addEntry(m, testRequest(1), true)
	r := testRequest(2)
	r.Action = "completion"
	r.Revision = 1
	response, err := m.Assist(context.Background(), r)
	if err != nil || !response.Pending || !response.Queued || response.QueueMS < 900 {
		t.Fatalf("queued response: %+v %v", response, err)
	}
	r.Action = "close"
	if _, err = m.Assist(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if len(m.waiters) != 0 {
		t.Fatal("close kept abandoned admission")
	}
	m.Close()
}

func TestAdmissionExpiresAbandonedTicket(t *testing.T) {
	m := testManager(1)
	m.waiters = []*admission{{key: "abandoned", retained: true, queuedAt: time.Now().Add(-time.Minute), lastSeen: time.Now().Add(-time.Minute)}}
	e, created, err := m.acquire(context.Background(), testRequest(2))
	if err != nil || !created || len(m.waiters) != 0 {
		t.Fatalf("abandoned head blocked admission: %v", err)
	}
	close(e.ready)
	m.finished(e)
	m.Close()
}

func TestFiveRetainedEditorsShareTwoSlots(t *testing.T) {
	m := testManager(2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errors := make(chan error, 5)
	for owner := int64(1); owner <= 5; owner++ {
		wg.Add(1)
		go func(owner int64) {
			defer wg.Done()
			e, created, err := m.acquire(ctx, testRequest(owner))
			if err != nil {
				errors <- err
				return
			}
			if !created {
				errors <- fmt.Errorf("unexpected shared session")
				return
			}
			m.mu.Lock()
			if m.live > 2 {
				errors <- fmt.Errorf("container budget exceeded: %d", m.live)
			}
			e.created = time.Now().Add(-pressureLease - time.Second)
			close(e.ready)
			m.mu.Unlock()
			// No close request: the next queued editor must reclaim this lease.
			m.finished(e)
		}(owner)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	m.Close()
}

func TestPendingProjectGetsExtraIdleGrace(t *testing.T) {
	m := testManager(2)
	e := addEntry(m, testRequest(1), false)
	e.created = time.Now()
	e.lastUsed = time.Now().Add(-2 * time.Second)
	e.projectPending = true
	if got := m.victimLocked(time.Now(), 2, false); got != "" {
		t.Fatal("still-loading project evicted before its next diagnostic poll")
	}
	e.projectPending = false
	if got := m.victimLocked(time.Now(), 2, false); got == "" {
		t.Fatal("ready idle project did not yield promptly")
	}
	m.Close()
}

// Block synthetic container cleanup at the same boundary as Docker removal.
// This exercises live-capacity accounting without executing any container code.
func holdCleanup(e *entry) chan struct{} {
	e.s = &session{}
	held := make(chan struct{})
	started := make(chan struct{})
	go e.s.closeOnce.Do(func() { close(started); <-held })
	<-started
	return held
}

func TestAdmissionParallelCleanupPreservesFIFOAndLiveBudget(t *testing.T) {
	m := testManager(3)
	old := make([]*entry, 3)
	held := make([]chan struct{}, 3)
	for i := range old {
		old[i] = addEntry(m, testRequest(int64(i+1)), true)
		held[i] = holdCleanup(old[i])
	}
	defer func() {
		for _, ch := range held {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
		m.Close()
	}()
	type result struct {
		e   *entry
		err error
	}
	results := make([]chan result, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for i := range results {
		results[i] = make(chan result, 1)
		go func(i int) {
			e, _, err := m.acquire(ctx, testRequest(int64(i+10)))
			results[i] <- result{e, err}
		}(i)
		waitQueue(t, m, i+1)
	}
	m.mu.Lock()
	for _, e := range old {
		e.users = 0
	}
	m.signalLocked()
	m.mu.Unlock()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		m.mu.Lock()
		retired, live := len(m.sessions) == 0, m.live
		m.mu.Unlock()
		if live != 3 {
			t.Fatal("cleanup released live capacity prematurely")
		}
		if retired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("independent cleanups were serialized")
		}
		time.Sleep(time.Millisecond)
	}
	for _, ch := range results {
		select {
		case <-ch:
			t.Fatal("replacement started before cleanup")
		default:
		}
	}
	// Only one removal completes: only the FIFO head may take that slot.
	close(held[0])
	first := <-results[0]
	if first.err != nil {
		t.Fatal(first.err)
	}
	close(first.e.ready)
	for _, ch := range results[1:] {
		select {
		case <-ch:
			t.Fatal("uncleaned slot admitted a replacement")
		default:
		}
	}
	close(held[1])
	close(held[2])
	m.finished(first.e)
	for _, ch := range results[1:] {
		r := <-ch
		if r.err != nil {
			t.Fatal(r.err)
		}
		close(r.e.ready)
		m.finished(r.e)
	}
	m.mu.Lock()
	live := m.live
	m.mu.Unlock()
	if live != 3 {
		t.Fatalf("live budget after handoff: %d", live)
	}
}

func TestReclaimDoesNotRetireMoreThanQueuedDemand(t *testing.T) {
	m := testManager(3)
	for i := int64(1); i <= 3; i++ {
		addEntry(m, testRequest(i), false)
	}
	m.mu.Lock()
	m.waiters = []*admission{{key: "next", owner: 10}}
	m.reclaimLocked(time.Now(), 10, false)
	remaining := len(m.sessions)
	m.mu.Unlock()
	if remaining != 2 {
		t.Fatalf("one waiter retired %d sessions", 3-remaining)
	}
	m.Close()
}
