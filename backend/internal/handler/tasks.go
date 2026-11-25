package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"devsec-contest01/internal/middleware"
	"devsec-contest01/internal/service"
	"devsec-contest01/internal/tasks"
	"devsec-contest01/internal/tasks/model"
)

type Tasks struct {
	registry  *tasks.Registry
	selection *service.Selection
}

func NewTasks(registry *tasks.Registry, selection *service.Selection) *Tasks {
	return &Tasks{registry: registry, selection: selection}
}

func tasksMeta(registry *tasks.Registry) []model.Description {
	out := make([]model.Description, 0)
	for _, task := range registry.List() {
		out = append(out, task.Description)
	}
	return out
}

func (h *Tasks) List(c *gin.Context) {
	c.JSON(http.StatusOK, tasksMeta(h.registry))
}

func (h *Tasks) Input(c *gin.Context) {
	taskID := c.Param("taskId")
	task, ok := h.registry.Get(taskID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown task"})
		return
	}

	participant := middleware.Participant(c)
	allowed, err := h.selection.CanAccess(c.Request.Context(), participant.ID, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if !allowed {
		c.JSON(http.StatusConflict, gin.H{"error": "task is not your active selection"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.txt"`, taskID))
	c.Data(http.StatusOK, "text/plain; charset=utf-8", task.Dataset)
}
