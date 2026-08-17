// Package middleware contains cross-cutting HTTP middlewares: structured
// access logging, panic recovery, request-id, and a simple in-process
// rate limiter.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// HeaderRequestID is the canonical request-id header name.
const HeaderRequestID = "X-Request-ID"

// RequestID assigns or propagates a request id and stores it on the context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(HeaderRequestID)
		if rid == "" {
			var b [12]byte
			_, _ = rand.Read(b[:])
			rid = hex.EncodeToString(b[:])
		}
		c.Writer.Header().Set(HeaderRequestID, rid)
		c.Set("request_id", rid)
		c.Next()
	}
}

// Logger emits one structured line per request. It deliberately omits
// bodies, headers containing the bot token, and any field that might
// contain PII (phone, name) in the log message.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		rid, _ := c.Get("request_id")
		dur := time.Since(start)
		status := c.Writer.Status()
		log.Info("http",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", status,
			"duration_ms", dur.Milliseconds(),
			"request_id", rid,
			"remote", c.ClientIP(),
		)
	}
}

// Recovery converts a panic into a 500 with a consistent error envelope
// while logging the panic server-side.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				rid, _ := c.Get("request_id")
				log.Error("panic recovered",
					"err", r,
					"path", c.FullPath(),
					"request_id", rid,
				)
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error": gin.H{
							"code":    "INTERNAL_ERROR",
							"message": "internal error",
						},
					})
				}
			}
		}()
		c.Next()
	}
}

// RateLimiter is a simple per-IP token-bucket. It is not distributed — for
// production-grade rate limiting the recommendation is to front the API
// with nginx/Cloudflare or a future Redis-backed limiter.
type RateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	rate      float64 // tokens per second
	burst     float64
	now       func() time.Time
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter returns a limiter that allows `burst` requests immediately
// and refills at `rate` tokens/sec per client key (IP).
func NewRateLimiter(rate, burst float64) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		now:     time.Now,
	}
}

func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	// Sweep stale entries occasionally.
	if now.Sub(l.lastSweep) > 5*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware returns a Gin handler that enforces the limiter.
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "too many requests",
				},
			})
			return
		}
		c.Next()
	}
}

// FromContext extracts a request id from a Gin-derived context. It returns
// "" if no id is present.
func FromContext(ctx context.Context) string {
	if c, ok := ctx.(*gin.Context); ok {
		if v, ok := c.Get("request_id"); ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}
