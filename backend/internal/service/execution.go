package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	dbsqlc "devsec-contest01/internal/db/sqlc"
	"devsec-contest01/internal/runner"
	"devsec-contest01/internal/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRunQueueFull = errors.New("execution queue full")
var ErrRunHistoryFull = errors.New("execution history budget reached")
var ErrRunRate = errors.New("execution rate limit")
var ErrRunNotFound = errors.New("execution not found")

// Includes running jobs. Only two workers execute; queued rows/source are bounded.
const maxOutstandingRuns = 32

// One public API instance owns this bounded queue. Results/source are durable;
// jobs interrupted by a backend restart are explicitly reported, never guessed.
type runRate struct {
	start time.Time
	count int
}
type Execution struct {
	rates     map[int64]runRate
	q         *dbsqlc.Queries
	pool      *pgxpool.Pool
	registry  *tasks.Registry
	selection *Selection
	client    *runner.Client
	mu        sync.Mutex
	queue     []dbsqlc.CodeRun
	owners    map[int64]bool
	active    map[[16]byte]bool
	workers   int
}

func NewExecution(pool *pgxpool.Pool, registry *tasks.Registry, selection *Selection, socket string) *Execution {
	return &Execution{q: dbsqlc.New(pool), pool: pool, registry: registry, selection: selection, client: runner.NewClient(socket), rates: make(map[int64]runRate), owners: make(map[int64]bool), active: make(map[[16]byte]bool)}
}
func (s *Execution) Create(ctx context.Context, owner int64, task, language, source string, requestID pgtype.UUID) (dbsqlc.CodeRun, error) {
	if !requestID.Valid || len(source) == 0 || len(source) > runner.MaxSource || (runner.Request{Language: language, Source: source, Task: task}).Validate() != nil {
		return dbsqlc.CodeRun{}, ErrInvalidInput
	}
	if _, ok := s.registry.Get(task); !ok {
		return dbsqlc.CodeRun{}, ErrTaskNotFound
	}
	existing, err := s.q.GetCodeRunByRequest(ctx, dbsqlc.GetCodeRunByRequestParams{ParticipantID: owner, RequestID: requestID})
	if err == nil {
		if existing.TaskID != task || existing.Language != language || existing.SourceCode != source {
			return dbsqlc.CodeRun{}, ErrInvalidInput
		}
		return s.Get(ctx, owner, existing.ID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.CodeRun{}, err
	}
	allowed, err := s.selection.CanAccess(ctx, owner, task)
	if err != nil {
		return dbsqlc.CodeRun{}, err
	}
	if !allowed {
		return dbsqlc.CodeRun{}, ErrTaskLocked
	}
	solved, err := s.selection.isSolved(ctx, owner, task)
	if err != nil {
		return dbsqlc.CodeRun{}, err
	}
	if solved {
		return dbsqlc.CodeRun{}, ErrTaskCompleted
	}
	counts, countErr := s.q.CodeRunCounts(ctx, owner)
	if countErr != nil {
		return dbsqlc.CodeRun{}, countErr
	}
	if counts.Total >= 2000 || counts.Personal >= 100 {
		return dbsqlc.CodeRun{}, ErrRunHistoryFull
	}
	s.mu.Lock()
	now := time.Now()
	rate := s.rates[owner]
	if now.Sub(rate.start) >= time.Minute {
		rate = runRate{start: now}
	}
	if rate.count >= 10 {
		s.mu.Unlock()
		return dbsqlc.CodeRun{}, ErrRunRate
	}
	for id, r := range s.rates {
		if now.Sub(r.start) >= time.Minute {
			delete(s.rates, id)
		}
	}
	if s.owners[owner] || len(s.owners) >= maxOutstandingRuns {
		s.mu.Unlock()
		return dbsqlc.CodeRun{}, ErrRunQueueFull
	}
	rate.count++
	s.rates[owner] = rate
	s.owners[owner] = true
	// Publish the durable row and its active marker under the same lock: an
	// idempotent retry must not mistake a newly inserted job for a restart.
	row, err := s.q.CreateCodeRun(ctx, dbsqlc.CreateCodeRunParams{ParticipantID: owner, RequestID: requestID, TaskID: task, Language: language, SourceCode: source})
	if err != nil {
		delete(s.owners, owner)
		s.mu.Unlock()
		return dbsqlc.CodeRun{}, err
	}
	s.active[row.ID.Bytes] = true
	s.queue = append(s.queue, row)
	for s.workers < 2 && len(s.queue) > 0 {
		s.workers++
		go s.work()
	}
	s.mu.Unlock()
	return row, nil
}
func (s *Execution) Get(ctx context.Context, owner int64, id pgtype.UUID) (dbsqlc.CodeRun, error) {
	row, err := s.q.GetCodeRun(ctx, dbsqlc.GetCodeRunParams{ID: id, ParticipantID: owner})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrRunNotFound
	}
	if err != nil {
		return row, err
	}
	if row.Status == "queued" || row.Status == "running" {
		s.mu.Lock()
		active := s.active[id.Bytes]
		s.mu.Unlock()
		if !active {
			updated, err := s.q.InterruptCodeRun(ctx, id)
			if err == nil {
				return updated, nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return row, err
			}
			return s.q.GetCodeRun(ctx, dbsqlc.GetCodeRunParams{ID: id, ParticipantID: owner})
		}
	}
	return row, nil
}
func (s *Execution) work() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.workers--
			s.mu.Unlock()
			return
		}
		job := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		s.process(job)
		s.mu.Lock()
		delete(s.active, job.ID.Bytes)
		delete(s.owners, job.ParticipantID)
		s.mu.Unlock()
	}
}
func (s *Execution) process(job dbsqlc.CodeRun) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	finish := dbsqlc.FinishCodeRunParams{ID: job.ID, Status: "system_error", Stdout: []byte{}, Stderr: []byte{}, Feedback: "Исполнитель недоступен. Проверьте запуск runner и настройку gVisor."}
	if err := s.q.StartCodeRun(ctx, job.ID); err != nil {
		return
	}
	task, ok := s.registry.Get(job.TaskID)
	if !ok {
		_, _ = s.q.FinishCodeRun(ctx, finish)
		return
	}
	result, err := s.client.Run(ctx, runner.Request{Language: job.Language, Source: job.SourceCode, Input: task.Dataset, Task: job.TaskID})
	if err != nil {
		if errors.Is(err, runner.ErrBusy) {
			finish.Feedback = "Исполнитель занят. Повторите запуск."
		}
		_, _ = s.q.FinishCodeRun(ctx, finish)
		return
	}
	finish.Status = result.Status
	finish.OutputTruncated = len(result.Stdout) > 65536 || len(result.Stderr) > 65536
	finish.Stdout = append([]byte{}, result.Stdout[:min(len(result.Stdout), 65536)]...)
	finish.Stderr = append([]byte{}, result.Stderr[:min(len(result.Stderr), 65536)]...)
	finish.ExitCode = int32(result.ExitCode)
	finish.DurationMs = result.DurationMS
	if result.Status != "ok" {
		finish.Feedback = executionFeedback(result.Status)
		_, _ = s.q.FinishCodeRun(ctx, finish)
		return
	}
	// Award points and finish the execution record atomically.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(context.Background())
	qtx := dbsqlc.New(tx)
	selection := NewSelection(qtx, s.registry)
	judge := NewJudge(qtx, s.registry, selection)
	verdict, err := judge.Submit(ctx, job.ParticipantID, job.TaskID, result.Stdout)
	if err != nil {
		finish.Status = "interrupted"
		finish.Feedback = "Задача уже решена или больше недоступна для отправки."
		_, _ = s.q.FinishCodeRun(ctx, finish)
		return
	}
	finish.Status = "wrong_answer"
	if verdict.Passed {
		finish.Status = "accepted"
	}
	finish.Passed = verdict.Passed
	finish.Earned = verdict.Earned
	finish.Feedback = verdict.Feedback
	if _, err = qtx.FinishCodeRun(ctx, finish); err != nil {
		return
	}
	_ = tx.Commit(ctx)
}
func executionFeedback(status string) string {
	switch status {
	case "compile_error":
		return "Ошибка компиляции или синтаксиса."
	case "compile_timeout":
		return "Превышено время компиляции (45 секунд)."
	case "runtime_error":
		return "Программа завершилась с ошибкой."
	case "time_limit":
		return "Превышено время выполнения."
	case "memory_limit":
		return "Превышен лимит памяти."
	case "output_limit":
		return "Превышен лимит вывода (1 МиБ)."
	}
	return fmt.Sprintf("Ошибка исполнителя: %s", status)
}
