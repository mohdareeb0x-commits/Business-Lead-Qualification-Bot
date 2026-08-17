package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

// Handler exposes liveness and readiness endpoints.
type Handler struct {
	mongo *mongo.Client
}

// New returns a Handler that uses m to verify the database on /ready.
func New(m *mongo.Client) *Handler { return &Handler{mongo: m} }

// Register binds /health and /ready onto r.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/health", h.live)
	r.GET("/ready", h.ready)
}

func (h *Handler) live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.mongo.Ping(ctx, nil); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "reason": "mongo"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
