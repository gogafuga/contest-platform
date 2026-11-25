package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"devsec-contest01/internal/service"
)

type Auth struct {
	svc *service.Auth
}

func NewAuth(svc *service.Auth) *Auth {
	return &Auth{svc: svc}
}

type registerRequest struct {
	Name  string `json:"name"`
	Group string `json:"group"`
}

type participantPayload struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

type registerResponse struct {
	Token       string             `json:"token"`
	Participant participantPayload `json:"participant"`
}

func (h *Auth) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	token, participant, err := h.svc.Register(c.Request.Context(), req.Name, req.Group)
	switch {
	case err == nil:
		c.JSON(http.StatusCreated, registerResponse{
			Token: token,
			Participant: participantPayload{
				ID:    participant.ID,
				Name:  participant.Name,
				Group: participant.Group,
			},
		})
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrDuplicateName):
		c.JSON(http.StatusConflict, gin.H{"error": "name already registered"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
