package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"devsec-contest01/internal/service"
	"devsec-contest01/internal/tasks"
)

type Leaderboard struct {
	svc      *service.Leaderboard
	registry *tasks.Registry
}

func NewLeaderboard(svc *service.Leaderboard, registry *tasks.Registry) *Leaderboard {
	return &Leaderboard{svc: svc, registry: registry}
}

type leaderboardEntryPayload struct {
	Name   string             `json:"name"`
	Group  string             `json:"group"`
	Scores map[string]float64 `json:"scores"`
	Total  float64            `json:"total"`
}

func (h *Leaderboard) List(c *gin.Context) {
	entries, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	payloadEntries := make([]leaderboardEntryPayload, 0, len(entries))
	for _, e := range entries {
		payloadEntries = append(payloadEntries, leaderboardEntryPayload{
			Name:   e.Name,
			Group:  e.Group,
			Scores: e.Scores,
			Total:  e.Total,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":   tasksMeta(h.registry),
		"entries": payloadEntries,
	})
}
