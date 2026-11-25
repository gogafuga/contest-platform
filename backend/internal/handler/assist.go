package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"devsec-contest01/internal/analyzer"
	"devsec-contest01/internal/middleware"
	"devsec-contest01/internal/service"
	"devsec-contest01/internal/tasks"
	"github.com/gin-gonic/gin"
)

type assistQuota struct {
	tokens float64
	last   time.Time
	active int
}
type Assist struct {
	client    *analyzer.Client
	selection *service.Selection
	registry  *tasks.Registry
	mu        sync.Mutex
	quotas    map[int64]*assistQuota
}

func NewAssist(socket string, selection *service.Selection, registry *tasks.Registry) *Assist {
	return &Assist{client: analyzer.NewClient(socket), selection: selection, registry: registry, quotas: make(map[int64]*assistQuota)}
}
func (h *Assist) acquire(id int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for id, q := range h.quotas {
		if q.active == 0 && now.Sub(q.last) > time.Minute {
			delete(h.quotas, id)
		}
	}
	q := h.quotas[id]
	if q == nil {
		q = &assistQuota{tokens: 12, last: now}
		h.quotas[id] = q
	}
	q.tokens = min(12, q.tokens+now.Sub(q.last).Seconds()*4)
	q.last = now
	if q.tokens < 1 || q.active >= 2 {
		return false
	}
	q.tokens--
	q.active++
	return true
}
func (h *Assist) release(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if q := h.quotas[id]; q != nil {
		q.active--
	}
}
func (h *Assist) Handle(c *gin.Context) {
	id := middleware.Participant(c).ID
	if !h.acquire(id) {
		c.Header("Retry-After", "1")
		c.JSON(429, gin.H{"error": "Подсказки временно заняты. Повторите запрос."})
		return
	}
	defer h.release(id)
	task := c.Param("taskId")
	if _, ok := h.registry.Get(task); !ok {
		c.JSON(404, gin.H{"error": "unknown task"})
		return
	}
	allowed, err := h.selection.CanAccess(c.Request.Context(), id, task)
	if err != nil {
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}
	if !allowed {
		c.JSON(403, gin.H{"error": "task is not accessible"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var request analyzer.AssistRequest
	decoder := json.NewDecoder(c.Request.Body)
	if decoder.Decode(&request) != nil || request.Validate() != nil {
		c.JSON(400, gin.H{"error": "invalid analyzer request"})
		return
	}
	// Identity and workspace ownership always come from the checked session/route.
	request.Owner = id
	request.Task = task
	ctx, cancel := context.WithTimeout(c.Request.Context(), 50*time.Second)
	defer cancel()
	response, err := h.client.Assist(ctx, request)
	switch {
	case err == nil:
		c.JSON(200, response)
	case errors.Is(err, analyzer.ErrBusy):
		c.Header("Retry-After", "2")
		c.JSON(429, gin.H{"error": "Все анализаторы заняты. Повторим запрос."})
	case errors.Is(err, analyzer.ErrStale):
		c.JSON(409, gin.H{"error": "editor revision changed"})
	default:
		c.JSON(503, gin.H{"error": "Сервис подсказок недоступен. Черновик продолжает сохраняться."})
	}
}
