package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "devsec-contest01/internal/db/sqlc"
)

// Admin-chosen duration bounds for a start/restart — loose sanity guards, not
// a product requirement: short enough contests aren't useful, absurdly long
// ones are almost certainly a typo (minutes entered as seconds, etc).
const (
	MinDurationSeconds = 60
	MaxDurationSeconds = 14 * 24 * 60 * 60
)

type ClockStatus string

const (
	StatusNotStarted ClockStatus = "not_started"
	StatusRunning    ClockStatus = "running"
	StatusEnded      ClockStatus = "ended"
)

type ClockState struct {
	StartedAt       *time.Time
	DurationSeconds int
	Status          ClockStatus
}

type Clock struct {
	q               dbsqlc.Querier
	durationSeconds int
}

func NewClock(q dbsqlc.Querier, durationSeconds int) *Clock {
	return &Clock{q: q, durationSeconds: durationSeconds}
}

// InitContestState seeds the singleton contest_state row on boot. A no-op if
// the row already exists, so a backend restart never silently resets the
// configured duration mid-contest.
func (c *Clock) InitContestState(ctx context.Context) error {
	return c.q.InitContestState(ctx, int32(c.durationSeconds))
}

func (c *Clock) Get(ctx context.Context) (ClockState, error) {
	row, err := c.q.GetContestState(ctx)
	if err != nil {
		return ClockState{}, err
	}
	return toClockState(row), nil
}

// Start begins the contest, or restarts it if already running/ended —
// StartContest always overwrites started_at. A nil durationSeconds keeps
// whatever duration is currently on record; a non-nil value overrides it
// (for this and every future start/restart, until overridden again).
func (c *Clock) Start(ctx context.Context, durationSeconds *int) (ClockState, error) {
	var param pgtype.Int4
	if durationSeconds != nil {
		d := *durationSeconds
		if d < MinDurationSeconds || d > MaxDurationSeconds {
			return ClockState{}, fmt.Errorf(
				"%w: duration must be between %d and %d seconds",
				ErrInvalidInput, MinDurationSeconds, MaxDurationSeconds,
			)
		}
		param = pgtype.Int4{Int32: int32(d), Valid: true}
	}

	row, err := c.q.StartContest(ctx, param)
	if err != nil {
		return ClockState{}, err
	}
	return toClockState(row), nil
}

func toClockState(row dbsqlc.ContestState) ClockState {
	state := ClockState{DurationSeconds: int(row.DurationSeconds)}
	if !row.StartedAt.Valid {
		state.Status = StatusNotStarted
		return state
	}

	startedAt := row.StartedAt.Time
	state.StartedAt = &startedAt
	if time.Since(startedAt) >= time.Duration(row.DurationSeconds)*time.Second {
		state.Status = StatusEnded
	} else {
		state.Status = StatusRunning
	}
	return state
}
