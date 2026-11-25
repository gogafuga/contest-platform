package handler

import (
	"devsec-contest01/internal/middleware"
	"devsec-contest01/internal/service"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

type Drafts struct {
	svc *service.Drafts
}

type draftResponse struct {
	Language   string     `json:"language"`
	SourceCode string     `json:"sourceCode"`
	Version    int64      `json:"version"`
	UpdatedAt  *time.Time `json:"updatedAt"`
}

type saveDraftRequest struct {
	Language   string `json:"language"`
	SourceCode string `json:"sourceCode"`
	Version    *int64 `json:"version"`
}
type draftHistoryEntry struct {
	Language  string    `json:"language"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
}
type draftHistorySnapshot struct {
	Language   string    `json:"language"`
	SourceCode string    `json:"sourceCode"`
	Version    int64     `json:"version"`
	CreatedAt  time.Time `json:"createdAt"`
}
type participantEventRequest struct {
	EventID   pgtype.UUID `json:"eventId"`
	EventType string      `json:"eventType"`
	ClientAt  time.Time   `json:"clientAt"`
}
type participantEventResponse struct {
	ID         int64     `json:"id"`
	EventType  string    `json:"eventType"`
	ClientAt   time.Time `json:"clientAt"`
	ReceivedAt time.Time `json:"receivedAt"`
}

func NewDrafts(svc *service.Drafts) *Drafts {
	return &Drafts{svc: svc}
}

func (h *Drafts) Get(c *gin.Context) {
	taskID := c.Param("taskId")
	participant := middleware.Participant(c)
	draft, err := h.svc.Get(c.Request.Context(), participant.ID, taskID)
	switch {
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
		return
	case errors.Is(err, service.ErrTaskLocked):
		c.JSON(http.StatusForbidden, gin.H{"error": "task is not accessible"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	var updatedAt *time.Time
	if draft.UpdatedAt.Valid {
		updatedAt = &draft.UpdatedAt.Time
	}

	c.JSON(http.StatusOK, draftResponse{
		SourceCode: draft.SourceCode,
		Language:   draft.Language,
		Version:    draft.Version,
		UpdatedAt:  updatedAt,
	})
}

func (h *Drafts) Save(c *gin.Context) {
	var req saveDraftRequest
	err := c.ShouldBindJSON(&req)
	if err != nil || req.Version == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	// Preserve compatibility with clients that predate language selection.
	if req.Language == "" {
		req.Language = "go"
	}
	taskID := c.Param("taskId")
	participant := middleware.Participant(c)

	draft, err := h.svc.Save(
		c.Request.Context(),
		participant.ID,
		taskID,
		req.SourceCode,
		req.Language,
		*req.Version,
	)
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	case errors.Is(err, service.ErrFileTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "source code too large"})
		return
	case errors.Is(err, service.ErrDraftConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "draft version conflict"})
		return
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
		return
	case errors.Is(err, service.ErrTaskLocked):
		c.JSON(http.StatusForbidden, gin.H{"error": "task is not accessible"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	var updatedAt *time.Time
	if draft.UpdatedAt.Valid {
		updatedAt = &draft.UpdatedAt.Time
	}

	c.JSON(http.StatusOK, draftResponse{
		SourceCode: draft.SourceCode,
		Language:   draft.Language,
		Version:    draft.Version,
		UpdatedAt:  updatedAt,
	})
}

func (h *Drafts) ListHistory(c *gin.Context) {
	participantID, err := strconv.ParseInt(c.Param("participantId"), 10, 64)
	if err != nil || participantID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid participant id"})
		return
	}
	taskID := c.Param("taskId")
	entries, err := h.svc.ListHistory(c.Request.Context(), participantID, taskID, 100)
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	response := make([]draftHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		response = append(response, draftHistoryEntry{
			Language:  entry.Language,
			Version:   entry.Version,
			CreatedAt: entry.CreatedAt.Time,
		})
	}
	c.JSON(http.StatusOK, response)
}

func (h *Drafts) GetHistoryVersion(c *gin.Context) {
	participantID, err := strconv.ParseInt(c.Param("participantId"), 10, 64)
	if err != nil || participantID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid participant id"})
		return
	}
	version, err := strconv.ParseInt(c.Param("version"), 10, 64)
	if err != nil || version <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version"})
		return
	}
	snapshot, err := h.svc.GetHistoryVersion(
		c.Request.Context(),
		participantID,
		c.Param("taskId"),
		version,
	)
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	case errors.Is(err, service.ErrTaskNotFound),
		errors.Is(err, service.ErrDraftVersionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "task or draft version not found"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, draftHistorySnapshot{
		Language:   snapshot.Language,
		SourceCode: snapshot.SourceCode,
		Version:    snapshot.Version,
		CreatedAt:  snapshot.CreatedAt.Time,
	})
}

func (h *Drafts) RecordEvent(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)

	var req participantEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event"})
		return
	}
	if req.ClientAt.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing event time"})
		return
	}

	participant := middleware.Participant(c)
	err := h.svc.RecordEvent(
		c.Request.Context(),
		participant.ID,
		c.Param("taskId"),
		req.EventID,
		req.EventType,
		pgtype.Timestamptz{Time: req.ClientAt, Valid: true},
	)

	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event"})
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
	case errors.Is(err, service.ErrTaskLocked):
		c.JSON(http.StatusForbidden, gin.H{"error": "task is not accessible"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	default:
		c.Status(http.StatusNoContent)
	}
}

func (h *Drafts) ListEvents(c *gin.Context) {
	participantID, err := strconv.ParseInt(c.Param("participantId"), 10, 64)
	if err != nil || participantID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid participant id"})
		return
	}

	afterID, err := strconv.ParseInt(c.DefaultQuery("afterId", "0"), 10, 64)
	if err != nil || afterID < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
		return
	}

	entries, err := h.svc.ListEvents(
		c.Request.Context(),
		participantID,
		c.Param("taskId"),
		afterID,
		100,
	)
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	response := make([]participantEventResponse, 0, len(entries))
	for _, entry := range entries {
		response = append(response, participantEventResponse{
			ID:         entry.ID,
			EventType:  entry.EventType,
			ClientAt:   entry.ClientAt.Time,
			ReceivedAt: entry.ReceivedAt.Time,
		})
	}
	c.JSON(http.StatusOK, response)
}

func (h *Drafts) SearchParticipants(c *gin.Context) {
	entries, err := h.svc.SearchParticipants(c.Request.Context(), c.Query("q"))
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "search query too long"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	response := make([]participantPayload, 0, len(entries))
	for _, entry := range entries {
		response = append(response, participantPayload{
			ID:    entry.ID,
			Name:  entry.Name,
			Group: entry.GroupName,
		})
	}
	c.JSON(http.StatusOK, response)
}

func (h *Drafts) GetTaskTiming(c *gin.Context) {
	participantID, err := strconv.ParseInt(c.Param("participantId"), 10, 64)
	if err != nil || participantID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid participant id"})
		return
	}
	timing, err := h.svc.GetTaskTiming(c.Request.Context(), participantID, c.Param("taskId"))
	switch {
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	default:
		c.JSON(http.StatusOK, timing)
	}
}
