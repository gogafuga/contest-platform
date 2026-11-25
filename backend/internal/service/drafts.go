package service

import (
	"context"
	dbsqlc "devsec-contest01/internal/db/sqlc"
	"devsec-contest01/internal/tasks"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Drafts struct {
	q         dbsqlc.Querier
	registry  *tasks.Registry
	selection *Selection
	pool      *pgxpool.Pool
}

const maxDraftBytes = 256 << 10

func NewDrafts(q dbsqlc.Querier, registry *tasks.Registry, selection *Selection, pool *pgxpool.Pool) *Drafts {
	return &Drafts{
		q:         q,
		registry:  registry,
		selection: selection,
		pool:      pool,
	}
}

func (s *Drafts) Get(ctx context.Context, participantID int64, taskID string) (dbsqlc.Draft, error) {
	_, ok := s.registry.Get(taskID)
	if ok == false {
		return dbsqlc.Draft{}, ErrTaskNotFound
	}
	allowed, err := s.selection.CanAccess(ctx, participantID, taskID)
	if err != nil {
		return dbsqlc.Draft{}, err
	}
	if allowed == false {
		return dbsqlc.Draft{}, ErrTaskLocked
	}
	draft, err := s.q.GetDraft(ctx, dbsqlc.GetDraftParams{
		ParticipantID: participantID,
		TaskID:        taskID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.Draft{
			ParticipantID: participantID,
			TaskID:        taskID,
			Language:      "go",
		}, nil
	}
	if err != nil {
		return dbsqlc.Draft{}, err
	}
	return draft, nil
}

func (s *Drafts) Save(ctx context.Context, participantID int64, taskID string, sourceCode string, language string, version int64) (dbsqlc.Draft, error) {
	if language != "go" && language != "python" && language != "csharp" {
		return dbsqlc.Draft{}, ErrInvalidInput
	}
	if len(sourceCode) > maxDraftBytes {
		return dbsqlc.Draft{}, ErrFileTooLarge
	}
	if version < 0 {
		return dbsqlc.Draft{}, ErrInvalidInput
	}
	_, ok := s.registry.Get(taskID)
	if ok == false {
		return dbsqlc.Draft{}, ErrTaskNotFound
	}
	allowed, err := s.selection.CanAccess(ctx, participantID, taskID)
	if err != nil {
		return dbsqlc.Draft{}, err
	}
	if allowed == false {
		return dbsqlc.Draft{}, ErrTaskLocked
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return dbsqlc.Draft{}, err
	}
	defer tx.Rollback(context.Background())

	qtx := dbsqlc.New(tx)
	var draft dbsqlc.Draft

	if version == 0 {
		draft, err = qtx.CreateDraft(ctx, dbsqlc.CreateDraftParams{
			ParticipantID: participantID,
			TaskID:        taskID,
			SourceCode:    sourceCode,
			Language:      language,
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return dbsqlc.Draft{}, ErrDraftConflict
		}
	} else {
		draft, err = qtx.UpdateDraft(ctx, dbsqlc.UpdateDraftParams{
			ParticipantID: participantID,
			TaskID:        taskID,
			SourceCode:    sourceCode,
			Language:      language,
			Version:       version,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return dbsqlc.Draft{}, ErrDraftConflict
		}
	}
	if err != nil {
		return dbsqlc.Draft{}, err
	}
	_, err = qtx.CreateDraftHistory(ctx, dbsqlc.CreateDraftHistoryParams{
		ParticipantID: draft.ParticipantID,
		TaskID:        draft.TaskID,
		SourceCode:    draft.SourceCode,
		Language:      draft.Language,
		Version:       draft.Version,
	})
	if err != nil {
		return dbsqlc.Draft{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return dbsqlc.Draft{}, err
	}

	return draft, nil
}

func (s *Drafts) ListHistory(ctx context.Context, participantID int64, taskID string, limit int32) ([]dbsqlc.ListDraftHistoryRow, error) {
	if participantID <= 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	_, ok := s.registry.Get(taskID)
	if ok == false {
		return []dbsqlc.ListDraftHistoryRow{}, ErrTaskNotFound
	}
	if !ok {
		return nil, ErrTaskNotFound
	}
	return s.q.ListDraftHistory(ctx, dbsqlc.ListDraftHistoryParams{
		ParticipantID: participantID,
		TaskID:        taskID,
		Limit:         limit,
	})
}

func (s *Drafts) GetHistoryVersion(ctx context.Context, participantID int64, taskID string, version int64) (dbsqlc.DraftHistory, error) {
	if participantID <= 0 || version <= 0 {
		return dbsqlc.DraftHistory{}, ErrInvalidInput
	}
	_, ok := s.registry.Get(taskID)
	if ok == false {
		return dbsqlc.DraftHistory{}, ErrTaskNotFound
	}
	snapshot, err := s.q.GetDraftHistoryVersion(ctx, dbsqlc.GetDraftHistoryVersionParams{
		ParticipantID: participantID,
		TaskID:        taskID,
		Version:       version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.DraftHistory{}, ErrDraftVersionNotFound
	}
	if err != nil {
		return dbsqlc.DraftHistory{}, err
	}
	return snapshot, nil
}

func (s *Drafts) RecordEvent(ctx context.Context, participantID int64, taskID string, eventID pgtype.UUID, eventType string, clientAt pgtype.Timestamptz) error {
	if !eventID.Valid || !clientAt.Valid ||
		clientAt.InfinityModifier != pgtype.Finite {
		return ErrInvalidInput
	}

	switch eventType {
	case "copy", "paste", "tab_hidden", "tab_visible":
	default:
		return ErrInvalidInput
	}

	if _, ok := s.registry.Get(taskID); !ok {
		return ErrTaskNotFound
	}

	allowed, err := s.selection.CanAccess(ctx, participantID, taskID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrTaskLocked
	}

	return s.q.CreateParticipantEvent(ctx, dbsqlc.CreateParticipantEventParams{
		ParticipantID: participantID,
		TaskID:        taskID,
		EventID:       eventID,
		EventType:     eventType,
		ClientAt:      clientAt,
	})
}

func (s *Drafts) ListEvents(ctx context.Context, participantID int64, taskID string, afterID int64, limit int32) ([]dbsqlc.ParticipantEvent, error) {
	if participantID <= 0 || afterID < 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	if _, ok := s.registry.Get(taskID); !ok {
		return nil, ErrTaskNotFound
	}

	return s.q.ListParticipantEvents(ctx, dbsqlc.ListParticipantEventsParams{
		ParticipantID: participantID,
		TaskID:        taskID,
		ID:            afterID,
		Limit:         limit,
	})
}

// SearchParticipants is exposed only through the administrative API.
func (s *Drafts) SearchParticipants(ctx context.Context, query string) ([]dbsqlc.SearchParticipantsRow, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 200 {
		return nil, ErrInvalidInput
	}
	return s.q.SearchParticipants(ctx, query)
}
