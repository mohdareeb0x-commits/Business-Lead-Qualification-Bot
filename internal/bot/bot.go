package bot

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/telegramleadbot/telegram-lead-bot/internal/apierr"
)

// Bot is the Gin-mounted webhook receiver. The webhook secret is checked
// against the X-Telegram-Bot-Api-Secret-Token header that Telegram sends
// when a secret token is configured for the webhook.
type Bot struct {
	handler *Handler
	secret  string
	maxBody int64
	log     *slog.Logger
}

// NewBot wires the HTTP-side bot.
func NewBot(h *Handler, webhookSecret string, log *slog.Logger) *Bot {
	return &Bot{
		handler: h,
		secret:  webhookSecret,
		maxBody: 1 << 20, // 1 MiB
		log:     log,
	}
}

// Register binds POST /api/v1/webhooks/telegram onto r.
func (b *Bot) Register(r gin.IRouter) {
	r.POST("/webhooks/telegram", b.handle)
}

func (b *Bot) handle(c *gin.Context) {
	if b.secret != "" {
		got := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(b.secret)) != 1 {
			apierr.Write(c, http.StatusUnauthorized, apierr.CodeUnauthorized, "invalid webhook secret", nil)
			return
		}
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, b.maxBody)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid body", nil)
		return
	}
	var u TelegramUpdate
	if err := json.Unmarshal(raw, &u); err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid update", nil)
		return
	}
	if u.UpdateID == 0 {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "missing update_id", nil)
		return
	}
	if u.Message == nil && u.CallbackQuery == nil {
		// Telegram requires 200 OK even for updates we choose to ignore,
		// otherwise it will keep retrying.
		c.Status(http.StatusOK)
		return
	}
	b.handler.HandleUpdate(c.Request.Context(), u)
	c.Status(http.StatusOK)
}

// ErrMissingUpdate indicates the body parsed but had no useful fields.
var ErrMissingUpdate = errors.New("no message or callback")
