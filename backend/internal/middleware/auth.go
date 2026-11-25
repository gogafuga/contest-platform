package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"devsec-contest01/internal/service"
)

const participantContextKey = "participant"

// ParticipantAuth resolves the Authorization: Bearer <token> header issued at
// registration to a participant, aborting with 401 if it's missing or invalid.
func ParticipantAuth(auth *service.Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		participant, err := auth.ValidateToken(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(participantContextKey, participant)
		c.Next()
	}
}

// Participant reads the participant set by ParticipantAuth. Only valid for
// handlers mounted behind that middleware.
func Participant(c *gin.Context) service.Participant {
	return c.MustGet(participantContextKey).(service.Participant)
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimPrefix(header, prefix)
}

func ParticipantEventLimit() gin.HandlerFunc {
	type window struct {
		start time.Time
		count int
	}

	var mu sync.Mutex
	windows := make(map[int64]window)

	return func(c *gin.Context) {
		now := time.Now()
		id := Participant(c).ID

		mu.Lock()

		// Удаляем истёкшие окна, чтобы карта не росла бесконечно.
		for participantID, w := range windows {
			if now.Sub(w.start) >= time.Minute {
				delete(windows, participantID)
			}
		}

		w, exists := windows[id]
		if !exists {
			w = window{start: now}
		}
		allowed := w.count < 120
		if allowed {
			w.count++
			windows[id] = w
		}

		mu.Unlock()

		if !allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(
				http.StatusTooManyRequests,
				gin.H{"error": "too many events"},
			)
			return
		}

		c.Next()
	}
}
