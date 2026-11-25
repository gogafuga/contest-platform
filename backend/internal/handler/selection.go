package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"devsec-contest01/internal/middleware"
	"devsec-contest01/internal/service"
)

type Selection struct {
	svc *service.Selection
}

func NewSelection(svc *service.Selection) *Selection {
	return &Selection{svc: svc}
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type selectionResponse struct {
	SelectedTaskID *string `json:"selectedTaskId"`
}

func (h *Selection) Select(c *gin.Context) {
	taskID := c.Param("taskId")
	participant := middleware.Participant(c)

	selected, err := h.svc.Select(c.Request.Context(), participant.ID, taskID)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, selectionResponse{SelectedTaskID: nullableString(selected)})
	case errors.Is(err, service.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
	case errors.Is(err, service.ErrTaskLocked):
		c.JSON(http.StatusConflict, gin.H{"error": "another task is already active"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

type meResponse struct {
	Participant    participantPayload `json:"participant"`
	SelectedTaskID *string            `json:"selectedTaskId"`
	Scores         map[string]float64 `json:"scores"`
}

func (h *Selection) Me(c *gin.Context) {
	participant := middleware.Participant(c)

	state, err := h.svc.GetState(c.Request.Context(), participant.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, meResponse{
		Participant: participantPayload{
			ID:    participant.ID,
			Name:  participant.Name,
			Group: participant.Group,
		},
		SelectedTaskID: nullableString(state.SelectedTaskID),
		Scores:         state.Scores,
	})
}
