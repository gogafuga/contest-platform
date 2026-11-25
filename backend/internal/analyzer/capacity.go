package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Capacity struct {
	MaxSessions      int  `json:"maxSessions"`
	LiveSessions     int  `json:"liveSessions"`
	Waiting          int  `json:"waiting"`
	RequestSlots     int  `json:"requestSlots"`
	AdmissionBlocked bool `json:"admissionBlocked"`
}

// Bound request buffers independently of containers; queue polls must not be
// rejected by an unrelated 16-request ceiling before reaching the FIFO queue.
func (m *Manager) RequestSlots() int { return maxWaiting + 2*m.MaxSessions }

func (m *Manager) Capacity() Capacity {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Capacity{MaxSessions: m.MaxSessions, LiveSessions: m.live, Waiting: len(m.waiters), RequestSlots: m.RequestSlots(), AdmissionBlocked: m.closed || m.ctx.Err() != nil || m.cleanupFailed.Load()}
}

// Private socket only; no source, owner IDs or credentials are exposed.
func (c *Client) Capacity(ctx context.Context) (Capacity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://analyzers/status", nil)
	if err != nil {
		return Capacity{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return Capacity{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Capacity{}, fmt.Errorf("capacity status %d", response.StatusCode)
	}
	var result Capacity
	err = json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&result)
	return result, err
}
