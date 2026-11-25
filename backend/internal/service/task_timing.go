package service

import (
	"context"
	"errors"
	"time"

	dbsqlc "devsec-contest01/internal/db/sqlc"
	"github.com/jackc/pgx/v5"
)

type TaskTiming struct {
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	ElapsedSeconds int64      `json:"elapsedSeconds"`
	Estimated      bool       `json:"estimated"`
}

func (s *Drafts) GetTaskTiming(ctx context.Context, participantID int64, taskID string) (TaskTiming, error) {
	if participantID <= 0 {
		return TaskTiming{}, ErrInvalidInput
	}
	task, ok := s.registry.Get(taskID)
	if !ok {
		return TaskTiming{}, ErrTaskNotFound
	}
	row, err := s.q.GetTaskTiming(ctx, dbsqlc.GetTaskTimingParams{
		ParticipantID: participantID,
		TaskID:        taskID,
		Points:        task.Points,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return TaskTiming{}, nil
	}
	if err != nil {
		return TaskTiming{}, err
	}
	result := TaskTiming{StartedAt: &row.StartedAt.Time, Estimated: row.Estimated}
	end := row.ObservedAt.Time
	if row.FinishedAt.Valid {
		result.FinishedAt = &row.FinishedAt.Time
		end = row.FinishedAt.Time
	}
	result.ElapsedSeconds = max(0, int64(end.Sub(row.StartedAt.Time).Seconds()))
	return result, nil
}
