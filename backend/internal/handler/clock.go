package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"devsec-contest01/internal/service"
)

type Clock struct {
	svc *service.Clock
}

func NewClock(svc *service.Clock) *Clock {
	return &Clock{svc: svc}
}

type clockResponse struct {
	StartedAt       *string `json:"startedAt"`
	DurationSeconds int     `json:"durationSeconds"`
	Status          string  `json:"status"`
}

func (h *Clock) Get(c *gin.Context) {
	state, err := h.svc.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, toClockResponse(state))
}

type startClockRequest struct {
	DurationSeconds *int `json:"durationSeconds"`
}

// Start begins or restarts the contest. The request body is optional — an
// empty body (the common case, from a plain "restart" click) keeps whatever
// duration is already on record; a body with durationSeconds overrides it.
func (h *Clock) Start(c *gin.Context) {
	var req startClockRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
	}

	state, err := h.svc.Start(c.Request.Context(), req.DurationSeconds)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, toClockResponse(state))
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

func toClockResponse(state service.ClockState) clockResponse {
	resp := clockResponse{
		DurationSeconds: state.DurationSeconds,
		Status:          string(state.Status),
	}
	if state.StartedAt != nil {
		formatted := state.StartedAt.UTC().Format(time.RFC3339)
		resp.StartedAt = &formatted
	}
	return resp
}
