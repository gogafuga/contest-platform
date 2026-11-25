package service

import (
	"context"
	"fmt"

	dbsqlc "devsec-contest01/internal/db/sqlc"
	"devsec-contest01/internal/tasks"
)

type JudgeResult struct {
	TaskID   string
	Passed   bool
	Feedback string
	Earned   float64
}

type Judge struct {
	q         dbsqlc.Querier
	registry  *tasks.Registry
	selection *Selection
}

func NewJudge(q dbsqlc.Querier, registry *tasks.Registry, selection *Selection) *Judge {
	return &Judge{q: q, registry: registry, selection: selection}
}

// Submit checks access and completion, validates output, and records full points or zero.
func (s *Judge) Submit(ctx context.Context, participantID int64, taskID string, content []byte) (JudgeResult, error) {
	task, ok := s.registry.Get(taskID)
	if !ok {
		return JudgeResult{}, ErrTaskNotFound
	}

	allowed, err := s.selection.CanAccess(ctx, participantID, taskID)
	if err != nil {
		return JudgeResult{}, err
	}
	if !allowed {
		return JudgeResult{}, ErrTaskLocked
	}

	solved, err := s.selection.isSolved(ctx, participantID, taskID)
	if err != nil {
		return JudgeResult{}, err
	}
	if solved {
		return JudgeResult{}, ErrTaskCompleted
	}

	verdict := task.Validate(content)
	earned := float64(0)
	if verdict.Passed {
		earned = task.Points
	}

	_, err = s.q.UpsertBestSubmission(ctx, dbsqlc.UpsertBestSubmissionParams{
		ParticipantID: participantID,
		TaskID:        taskID,
		Score:         earned,
	})
	if err != nil {
		return JudgeResult{}, fmt.Errorf("upsert submission: %w", err)
	}

	return JudgeResult{TaskID: taskID, Passed: verdict.Passed, Feedback: verdict.Feedback, Earned: earned}, nil
}
