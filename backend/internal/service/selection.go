package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "devsec-contest01/internal/db/sqlc"
	"devsec-contest01/internal/tasks"
)

// ParticipantState is the server-side source of truth for what a participant
// has selected and scored, used to restore frontend state after a reload.
type ParticipantState struct {
	SelectedTaskID string // "" if nothing selected yet
	Scores         map[string]float64
}

// Selection enforces the single-active-task rule: a participant works on
// exactly one task at a time, and can only switch once the current one is
// solved for full points. Solved tasks stay accessible even after switching.
type Selection struct {
	q        dbsqlc.Querier
	registry *tasks.Registry
}

func NewSelection(q dbsqlc.Querier, registry *tasks.Registry) *Selection {
	return &Selection{q: q, registry: registry}
}

// Select attempts to lock the participant onto taskID, returning the
// resulting selection (always taskID on success).
func (s *Selection) Select(ctx context.Context, participantID int64, taskID string) (string, error) {
	if _, ok := s.registry.Get(taskID); !ok {
		return "", ErrTaskNotFound
	}

	p, err := s.q.GetParticipantByID(ctx, participantID)
	if err != nil {
		return "", err
	}
	current := ""
	if p.SelectedTaskID.Valid {
		current = p.SelectedTaskID.String
	}

	if needsSolvedCheck(current, taskID) {
		solved, err := s.isSolved(ctx, participantID, current)
		if err != nil {
			return "", err
		}
		if !solved {
			return "", ErrTaskLocked
		}
	}

	updated, err := s.q.SetSelectedTask(ctx, dbsqlc.SetSelectedTaskParams{
		ID:             participantID,
		SelectedTaskID: pgtype.Text{String: taskID, Valid: true},
	})
	if err != nil {
		return "", err
	}
	return updated.SelectedTaskID.String, nil
}

// CanAccess reports whether the participant may download input or submit for
// taskID right now: it's their current selection, or it's already solved.
func (s *Selection) CanAccess(ctx context.Context, participantID int64, taskID string) (bool, error) {
	p, err := s.q.GetParticipantByID(ctx, participantID)
	if err != nil {
		return false, err
	}
	if p.SelectedTaskID.Valid && p.SelectedTaskID.String == taskID {
		return true, nil
	}
	return s.isSolved(ctx, participantID, taskID)
}

// GetState returns the participant's current selection and per-task best
// scores, for restoring frontend state after a reload.
func (s *Selection) GetState(ctx context.Context, participantID int64) (ParticipantState, error) {
	row, err := s.q.GetParticipantState(ctx, participantID)
	if err != nil {
		return ParticipantState{}, err
	}
	selected := ""
	if row.SelectedTaskID.Valid {
		selected = row.SelectedTaskID.String
	}
	return ParticipantState{SelectedTaskID: selected, Scores: scoresFromJSONB(row.Scores)}, nil
}

func (s *Selection) isSolved(ctx context.Context, participantID int64, taskID string) (bool, error) {
	task, ok := s.registry.Get(taskID)
	if !ok {
		return false, nil
	}
	score, err := s.q.GetSubmissionScore(ctx, dbsqlc.GetSubmissionScoreParams{
		ParticipantID: participantID,
		TaskID:        taskID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return score >= task.Points, nil
}

// needsSolvedCheck reports whether switching from current to taskID is a
// real switch that requires current to already be solved — i.e. it's not a
// no-op (nothing selected yet, or re-selecting the same task).
func needsSolvedCheck(current, taskID string) bool {
	return current != "" && current != taskID
}
