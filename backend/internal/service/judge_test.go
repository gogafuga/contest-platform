package service

import (
	"context"
	"errors"
	"os"
	"testing"

	dbsqlc "devsec-contest01/internal/db/sqlc"
	"devsec-contest01/internal/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Embed Querier so any unexpected DB call fails rather than silently succeeding.
type judgeDB struct {
	dbsqlc.Querier
	selected string
	score    float64
	saved    []float64
}

func (q *judgeDB) GetParticipantByID(context.Context, int64) (dbsqlc.Participant, error) {
	return dbsqlc.Participant{SelectedTaskID: pgtype.Text{String: q.selected, Valid: true}}, nil
}
func (q *judgeDB) GetSubmissionScore(context.Context, dbsqlc.GetSubmissionScoreParams) (float64, error) {
	if q.score == 0 {
		return 0, pgx.ErrNoRows
	}
	return q.score, nil
}
func (q *judgeDB) UpsertBestSubmission(_ context.Context, p dbsqlc.UpsertBestSubmissionParams) (dbsqlc.Submission, error) {
	q.saved = append(q.saved, p.Score)
	q.score = p.Score
	return dbsqlc.Submission{Score: p.Score}, nil
}

func TestJudgeRetryAndCompletion(t *testing.T) {
	ctx := context.Background()
	registry := tasks.NewRegistry()
	q := &judgeDB{selected: "password-validator"}
	judge := NewJudge(q, registry, NewSelection(q, registry))
	failed, err := judge.Submit(ctx, 1, q.selected, []byte("wrong"))
	if err != nil || failed.Passed || failed.Earned != 0 {
		t.Fatalf("failed submission: %+v, %v", failed, err)
	}
	answer, err := os.ReadFile("../tasks/task_02/data/expected.txt")
	if err != nil {
		t.Fatal(err)
	}
	passed, err := judge.Submit(ctx, 1, q.selected, answer)
	if err != nil || !passed.Passed || passed.Earned != 100 {
		t.Fatalf("retry: %+v, %v", passed, err)
	}
	// Completed tasks stay accessible after moving to another selection.
	q.selected = "cidr-check"
	_, err = judge.Submit(ctx, 1, "password-validator", []byte("wrong"))
	if !errors.Is(err, ErrTaskCompleted) || len(q.saved) != 2 {
		t.Fatalf("completed task: %v, writes=%v", err, q.saved)
	}
}

func TestJudgeRejectsUnknownAndLockedTasks(t *testing.T) {
	registry := tasks.NewRegistry()
	q := &judgeDB{selected: "noisy-ips"}
	judge := NewJudge(q, registry, NewSelection(q, registry))
	for _, tc := range []struct {
		id   string
		want error
	}{
		{"missing", ErrTaskNotFound}, {"password-validator", ErrTaskLocked},
	} {
		_, err := judge.Submit(context.Background(), 1, tc.id, nil)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.id, err, tc.want)
		}
	}
	if len(q.saved) != 0 {
		t.Fatal("rejected submissions were saved")
	}
}
