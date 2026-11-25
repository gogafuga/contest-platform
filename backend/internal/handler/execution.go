package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	dbsqlc "devsec-contest01/internal/db/sqlc"
	"devsec-contest01/internal/middleware"
	"devsec-contest01/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

type Execution struct{ svc *service.Execution }

func NewExecution(svc *service.Execution) *Execution { return &Execution{svc: svc} }

type runResponse struct {
	OutputTruncated bool        `json:"outputTruncated"`
	ID              pgtype.UUID `json:"id"`
	Status          string      `json:"status"`
	Language        string      `json:"language"`
	Stdout          string      `json:"stdout"`
	Stderr          string      `json:"stderr"`
	ExitCode        int32       `json:"exitCode"`
	DurationMS      int64       `json:"durationMs"`
	Passed          bool        `json:"passed"`
	Earned          float64     `json:"earned"`
	Feedback        string      `json:"feedback"`
	CreatedAt       time.Time   `json:"createdAt"`
}

func runPayload(row dbsqlc.CodeRun) runResponse {
	return runResponse{OutputTruncated: row.OutputTruncated, ID: row.ID, Status: row.Status, Language: row.Language, Stdout: string(row.Stdout), Stderr: string(row.Stderr), ExitCode: row.ExitCode, DurationMS: row.DurationMs, Passed: row.Passed, Earned: row.Earned, Feedback: row.Feedback, CreatedAt: row.CreatedAt.Time}
}
func (h *Execution) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var request struct {
		RequestID pgtype.UUID `json:"requestId"`
		Language  string      `json:"language"`
		Source    string      `json:"sourceCode"`
	}
	if json.NewDecoder(c.Request.Body).Decode(&request) != nil {
		c.JSON(400, gin.H{"error": "invalid run request"})
		return
	}
	row, err := h.svc.Create(c.Request.Context(), middleware.Participant(c).ID, c.Param("taskId"), request.Language, request.Source, request.RequestID)
	if err == nil {
		c.JSON(http.StatusAccepted, runPayload(row))
		return
	}
	runError(c, err)
}
func (h *Execution) Get(c *gin.Context) {
	var id pgtype.UUID
	if id.Scan(c.Param("runId")) != nil || !id.Valid {
		c.JSON(400, gin.H{"error": "invalid run id"})
		return
	}
	row, err := h.svc.Get(c.Request.Context(), middleware.Participant(c).ID, id)
	if err == nil && row.TaskID != c.Param("taskId") {
		err = service.ErrRunNotFound
	}
	if err == nil {
		c.JSON(200, runPayload(row))
		return
	}
	runError(c, err)
}
func runError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(400, gin.H{"error": "Проверьте исходный код, язык и идентификатор запуска."})
	case errors.Is(err, service.ErrTaskNotFound), errors.Is(err, service.ErrRunNotFound):
		c.JSON(404, gin.H{"error": "Запуск или задача не найдены."})
	case errors.Is(err, service.ErrTaskLocked):
		c.JSON(403, gin.H{"error": "Задача недоступна."})
	case errors.Is(err, service.ErrTaskCompleted):
		c.JSON(409, gin.H{"error": "Задача уже решена."})
	case errors.Is(err, service.ErrRunHistoryFull):
		c.JSON(429, gin.H{"error": "Достигнут лимит истории запусков. Обратитесь к преподавателю."})
	case errors.Is(err, service.ErrRunRate):
		c.Header("Retry-After", "60")
		c.JSON(429, gin.H{"error": "Не более 10 запусков в минуту."})
	case errors.Is(err, service.ErrRunQueueFull):
		c.Header("Retry-After", "2")
		c.JSON(429, gin.H{"error": "Уже есть активный запуск или очередь заполнена."})
	default:
		c.JSON(500, gin.H{"error": "Не удалось создать запуск. Проверьте миграцию 006 и доступность базы."})
	}
}
