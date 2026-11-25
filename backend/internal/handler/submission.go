package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"devsec-contest01/internal/middleware"
	"devsec-contest01/internal/service"
)

const maxSubmissionBytes = 2 << 20 // 2 MiB

type Submission struct {
	svc *service.Judge
}

func NewSubmission(svc *service.Judge) *Submission {
	return &Submission{svc: svc}
}

type submissionResponse struct {
	TaskID   string  `json:"taskId"`
	Passed   bool    `json:"passed"`
	Feedback string  `json:"feedback"`
	Earned   float64 `json:"earned"`
}

func (h *Submission) Submit(c *gin.Context) {
	taskID := c.Param("taskId")

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file field"})
		return
	}
	if fileHeader.Size > maxSubmissionBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file too large"})
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read file"})
		return
	}
	defer f.Close()

	content, err := io.ReadAll(io.LimitReader(f, maxSubmissionBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read file"})
		return
	}
	if len(content) > maxSubmissionBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file too large"})
		return
	}

	participant := middleware.Participant(c)
	result, err := h.svc.Submit(c.Request.Context(), participant.ID, taskID, content)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, submissionResponse{
			TaskID:   result.TaskID,
			Passed:   result.Passed,
			Feedback: result.Feedback,
			Earned:   result.Earned,
		})
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
	case errors.Is(err, service.ErrTaskCompleted):
		c.JSON(http.StatusConflict, gin.H{"error": "Задача уже решена. Повторная отправка недоступна."})
	case errors.Is(err, service.ErrTaskLocked):
		c.JSON(http.StatusConflict, gin.H{"error": "task is not your active selection"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
