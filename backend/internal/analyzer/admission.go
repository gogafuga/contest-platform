package analyzer

import (
	"context"
	"time"
)

const maxWaiting = 32
const pressureIdle = time.Second
const pendingIdle = 3 * time.Second
const pressureLease = 20 * time.Second

const admissionPoll = time.Second
const admissionTTL = 5 * time.Second

type admission struct {
	key      string
	owner    int64
	queuedAt time.Time
	lastSeen time.Time
	requests int
	retained bool
	canceled bool
}

// All queue/session bookkeeping is protected by m.mu. Retiring containers still
// count toward live: no replacement can exceed the configured resource budget.
func (m *Manager) signalLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

func (m *Manager) dequeueLocked(ticket *admission) {
	for i, waiting := range m.waiters {
		if waiting == ticket {
			m.waiters = append(m.waiters[:i], m.waiters[i+1:]...)
			m.signalLocked()
			return
		}
	}
}

// Polls preserve the same bounded FIFO ticket, but not source code or tokens.
// Queue time does not consume the request budget for cold initialization.
func (m *Manager) acquire(ctx context.Context, r AssistRequest) (*entry, bool, error) {
	k := key(r)
	var ticket *admission
	poll := time.NewTimer(admissionPoll)
	defer poll.Stop()
	defer func() {
		if ticket != nil {
			m.mu.Lock()
			ticket.requests--
			if ticket.requests == 0 && !ticket.retained {
				m.dequeueLocked(ticket)
			}
			m.mu.Unlock()
		}
	}()
	for {
		m.mu.Lock()
		if m.closed || m.ctx.Err() != nil || m.cleanupFailed.Load() {
			m.mu.Unlock()
			return nil, false, ErrUnavailable
		}
		if ctx.Err() != nil || (ticket != nil && ticket.canceled) {
			m.mu.Unlock()
			return nil, false, ErrBusy
		}
		now := time.Now()
		m.pruneWaitersLocked(now)
		if e := m.sessions[k]; e != nil {
			e.users++
			m.mu.Unlock()
			return e, false, nil
		}
		if ticket == nil {
			for _, waiting := range m.waiters {
				if waiting.key == k {
					ticket = waiting
					ticket.requests++
					ticket.lastSeen = now
					break
				}
			}
		}
		owned, waitingOwned := 0, 0
		for _, e := range m.sessions {
			if e.owner == r.Owner {
				owned++
			}
		}
		for _, waiting := range m.waiters {
			if waiting.owner == r.Owner {
				waitingOwned++
			}
		}
		first := len(m.waiters) == 0 || m.waiters[0] == ticket
		if first && m.live < m.MaxSessions && owned < 2 {
			e := &entry{ready: make(chan struct{}), owner: r.Owner, users: 1, created: now, lastUsed: now}
			if ticket != nil {
				e.admissionWait = now.Sub(ticket.queuedAt)
			}
			m.sessions[k] = e
			m.live++
			m.dequeueLocked(ticket)
			m.mu.Unlock()
			return e, true, nil
		}
		if ticket == nil {
			if len(m.waiters) >= maxWaiting || waitingOwned >= 2 {
				m.mu.Unlock()
				return nil, false, ErrBusy
			}
			ticket = &admission{key: k, owner: r.Owner, queuedAt: now, lastSeen: now, requests: 1}
			m.waiters = append(m.waiters, ticket)
		}
		if m.waiters[0] == ticket {
			m.reclaimLocked(now, r.Owner, owned >= 2)
		}
		changed := m.changed
		m.mu.Unlock()
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
		case <-changed:
		case <-timer.C:
		case <-poll.C:
			timer.Stop()
			m.mu.Lock()
			if ctx.Err() != nil || ticket.canceled {
				m.mu.Unlock()
				return nil, false, ErrBusy
			}
			if m.closed || m.cleanupFailed.Load() {
				m.mu.Unlock()
				return nil, false, ErrUnavailable
			}
			ticket.retained = true
			ticket.lastSeen = time.Now()
			m.mu.Unlock()
			return nil, false, errQueued
		}
		timer.Stop()
	}
}

func (m *Manager) pruneWaitersLocked(now time.Time) {
	for i := 0; i < len(m.waiters); {
		ticket := m.waiters[i]
		if ticket.requests == 0 && now.Sub(ticket.lastSeen) > admissionTTL {
			ticket.canceled = true
			m.waiters = append(m.waiters[:i], m.waiters[i+1:]...)
			m.signalLocked()
		} else {
			i++
		}
	}
}

func (m *Manager) queueElapsed(k string) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ticket := range m.waiters {
		if ticket.key == k {
			return time.Since(ticket.queuedAt)
		}
	}
	return 0
}

// Retire independent idle slots together rather than serializing Docker cleanup.
// Missing map entries include both free and retiring slots: count them once,
// and never admit their replacements until release confirms cleanup completed.
func (m *Manager) reclaimLocked(now time.Time, owner int64, ownOnly bool) {
	if ownOnly {
		// An owner's third editor must make room within its own two-session cap.
		if victim := m.victimLocked(now, owner, true); victim != "" {
			m.retireLocked(victim)
		}
		return
	}
	needed := len(m.waiters) - (m.MaxSessions - len(m.sessions))
	for ; needed > 0; needed-- {
		victim := m.victimLocked(now, owner, false)
		if victim == "" {
			return
		}
		m.retireLocked(victim)
	}
}

func (m *Manager) victimLocked(now time.Time, owner int64, ownOnly bool) string {
	var victim string
	var oldest time.Time
	for k, e := range m.sessions {
		if e.users != 0 || (ownOnly && e.owner != owner) {
			continue
		}
		select {
		case <-e.ready:
		default:
			continue
		}
		idle := pressureIdle
		if e.projectPending {
			idle = pendingIdle
		}
		if now.Sub(e.lastUsed) < idle && now.Sub(e.created) < pressureLease {
			continue
		}
		if victim == "" || e.lastUsed.Before(oldest) {
			victim, oldest = k, e.lastUsed
		}
	}
	return victim
}

func (m *Manager) finished(e *entry) {
	m.mu.Lock()
	e.users--
	e.lastUsed = time.Now()
	m.signalLocked()
	m.mu.Unlock()
}

// Caller holds m.mu. Entry identity checks in removeIf prevent a late callback
// from deleting a replacement session with the same editor key.
func (m *Manager) retireLocked(k string) {
	e := m.sessions[k]
	if e == nil {
		return
	}
	delete(m.sessions, k)
	m.cleanup.Add(1)
	go func() {
		defer m.cleanup.Done()
		defer m.release(e)
		<-e.ready
		if e.s != nil {
			m.closeSession(e.s)
		}
	}()
}

func (m *Manager) removeIf(k string, expected *entry) {
	m.mu.Lock()
	if expected == nil || m.sessions[k] == expected {
		m.retireLocked(k)
		if expected == nil {
			for i := 0; i < len(m.waiters); {
				ticket := m.waiters[i]
				if ticket.key == k {
					ticket.canceled = true
					m.dequeueLocked(ticket)
				} else {
					i++
				}
			}
		}
	}
	m.mu.Unlock()
}

func (m *Manager) expire() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneWaitersLocked(time.Now())
	for k, e := range m.sessions {
		if e.users != 0 {
			continue
		}
		select {
		case <-e.ready:
			if e.s != nil && (e.s.ctx.Err() != nil || time.Since(e.lastUsed) > 90*time.Second) {
				m.retireLocked(k)
			}
		default:
		}
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, ticket := range m.waiters {
		ticket.canceled = true
	}
	m.waiters = nil
	for k := range m.sessions {
		m.retireLocked(k)
	}
	m.signalLocked()
	m.mu.Unlock()
	m.cleanup.Wait()
}
