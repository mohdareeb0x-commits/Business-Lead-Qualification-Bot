// Package notification delivers admin-facing messages over Telegram.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/telegramleadbot/telegram-lead-bot/internal/lead"
)

// Telegram posts messages to a single admin chat via the Bot HTTP API.
// It implements lead.Notifier and is safe to call from request handlers.
type Telegram struct {
	token    string
	chatID   int64
	endpoint string
	client   *http.Client
}

// NewTelegram builds a notifier. token may be empty — NotifyNewLead will
// then return an error and the caller will log it.
func NewTelegram(token string, chatID int64) *Telegram {
	return &Telegram{
		token:    strings.TrimSpace(token),
		chatID:   chatID,
		endpoint: "https://api.telegram.org",
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// sendMessageRequest is a tiny subset of the sendMessage API we use.
type sendMessageRequest struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// sendMessageResponse captures whether Telegram accepted the message.
type sendMessageResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		MessageID int `json:"message_id"`
	} `json:"result"`
	Description string `json:"description"`
}

// NotifyNewLead sends a formatted lead summary to the admin chat. It is
// best-effort: errors are returned to the caller which decides whether
// the failure should block the user-facing flow.
func (t *Telegram) NotifyNewLead(ctx context.Context, l *lead.Lead) error {
	if t == nil || t.token == "" {
		return fmt.Errorf("telegram token not configured")
	}
	body := sendMessageRequest{
		ChatID:    t.chatID,
		Text:      FormatLead(l),
		ParseMode: "MarkdownV2",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", t.endpoint, t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var ok sendMessageResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&ok); err == nil && ok.OK {
			return nil
		}
		// Fall through to error path.
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return fmt.Errorf("telegram sendMessage failed: status=%d body=%s", resp.StatusCode, string(raw))
}

// FormatLead renders a lead for the admin notification using MarkdownV2-safe
// escaping. Reserved MarkdownV2 characters are escaped so the message
// renders correctly.
func FormatLead(l *lead.Lead) string {
	emoji := "🟢"
	switch l.Temperature {
	case lead.TemperatureHot:
		emoji = "🔥"
	case lead.TemperatureWarm:
		emoji = "🟡"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s *NEW LEAD*\n\n", emoji)
	fmt.Fprintf(&b, "*Name:* %s\n", esc(l.Name))
	fmt.Fprintf(&b, "*Phone:* %s\n", esc(l.Phone))
	if l.TelegramUsername != "" {
		fmt.Fprintf(&b, "*Telegram:* @%s\n", esc(l.TelegramUsername))
	}
	fmt.Fprintf(&b, "\n*Service:* %s\n", esc(l.Service))
	if l.Requirements.PropertyType != "" {
		fmt.Fprintf(&b, "*Property:* %s\n", esc(l.Requirements.PropertyType))
	}
	if l.Budget != "" {
		fmt.Fprintf(&b, "*Budget:* %s\n", esc(l.Budget))
	}
	if l.Location != "" {
		fmt.Fprintf(&b, "*Location:* %s\n", esc(l.Location))
	}
	if l.Timeline != "" {
		fmt.Fprintf(&b, "*Timeline:* %s\n", esc(l.Timeline))
	}
	fmt.Fprintf(&b, "\n*Score:* %d/100\n", l.Score)
	fmt.Fprintf(&b, "*Temperature:* %s\n", esc(string(l.Temperature)))
	fmt.Fprintf(&b, "*Status:* %s\n", esc(string(l.Status)))
	fmt.Fprintf(&b, "*Source:* %s\n", esc(string(l.Source)))
	return b.String()
}

// esc escapes the MarkdownV2 reserved characters in s so it can be safely
// embedded in a MarkdownV2 message body.
func esc(s string) string {
	r := strings.NewReplacer(
		`_`, `\_`, `*`, `\*`, `[`, `\[`, `]`, `\]`,
		`(`, `\(`, `)`, `\)`, `~`, `\~`, `>`, `\>`,
		`#`, `\#`, `+`, `\+`, `-`, `\-`, `=`, `\=`,
		`|`, `\|`, `{`, `\{`, `}`, `\}`, `.`, `\.`,
		`!`, `\!`, "`", "\\`",
	)
	return r.Replace(s)
}
