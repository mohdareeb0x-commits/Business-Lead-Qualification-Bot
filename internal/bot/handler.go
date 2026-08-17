package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/telegramleadbot/telegram-lead-bot/internal/lead"
)

// TelegramUpdate is the subset of the official Update shape we consume.
type TelegramUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *TelegramMessage `json:"message,omitempty"`
	CallbackQuery *CallbackQuery   `json:"callback_query,omitempty"`
}

// TelegramMessage is the subset of Message we read.
type TelegramMessage struct {
	MessageID int64    `json:"message_id"`
	From      *User    `json:"from,omitempty"`
	Chat      Chat     `json:"chat"`
	Text      string   `json:"text,omitempty"`
	Contact   *Contact `json:"contact,omitempty"`
}

// Contact is a Telegram contact payload.
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
}

// User is the minimal sender profile we read.
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

// Chat identifies the chat a message came from / is sent to.
type Chat struct {
	ID int64 `json:"id"`
}

// CallbackQuery represents a button click.
type CallbackQuery struct {
	ID      string           `json:"id"`
	From    *User            `json:"from,omitempty"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
}

// Handler is the bot's HTTP-side entrypoint. It validates the webhook secret,
// parses the update, drives the conversation state machine, and dispatches
// outbound messages.
type Handler struct {
	token    string
	store    Store
	svc      *lead.Service
	log      *slog.Logger
	endpoint string
	client   *http.Client
	now      func() time.Time
}

// NewHandler wires dependencies. token may be empty — in that case the
// handler will still parse updates but skip outbound calls.
func NewHandler(token string, store Store, svc *lead.Service, log *slog.Logger) *Handler {
	return &Handler{
		token:    strings.TrimSpace(token),
		store:    store,
		svc:      svc,
		log:      log,
		endpoint: "https://api.telegram.org",
		client:   &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
	}
}

// telegramResponse is the minimal subset of the API response.
type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

// sendMessageRequest is the payload to sendMessage.
type sendMessageRequest struct {
	ChatID      int64       `json:"chat_id"`
	Text        string      `json:"text"`
	ParseMode   string      `json:"parse_mode,omitempty"`
	ReplyMarkup interface{} `json:"reply_markup,omitempty"`
}

// answerCallbackRequest is the payload to answerCallbackQuery.
type answerCallbackRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
}

// HandleUpdate is the entrypoint invoked from the HTTP webhook.
func (h *Handler) HandleUpdate(ctx context.Context, u TelegramUpdate) {
	if u.CallbackQuery != nil {
		h.handleCallback(ctx, u.CallbackQuery)
		return
	}
	if u.Message != nil {
		h.handleMessage(ctx, u.Message)
	}
}

// send posts text + optional reply markup to a chat.
func (h *Handler) send(ctx context.Context, chatID int64, text string, markup interface{}) error {
	if h.token == "" {
		h.log.Warn("telegram token empty; skipping send", "chat_id", chatID)
		return nil
	}
	body := sendMessageRequest{ChatID: chatID, Text: text, ReplyMarkup: markup}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", h.endpoint, h.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("telegram send failed: status=%d body=%s", resp.StatusCode, string(raw))
	}
	return nil
}

// answerCallback acknowledges a button press (removes the "loading" indicator).
func (h *Handler) answerCallback(ctx context.Context, callbackID, text string) error {
	if h.token == "" {
		return nil
	}
	body := answerCallbackRequest{CallbackQueryID: callbackID, Text: text}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/answerCallbackQuery", h.endpoint, h.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// handleMessage routes a Telegram message to the right state transition.
func (h *Handler) handleMessage(ctx context.Context, m *TelegramMessage) {
	if m.From == nil {
		return
	}
	c, _ := h.store.Get(m.From.ID)
	if c == nil {
		c = &Conversation{TelegramID: m.From.ID, TelegramUsername: m.From.Username, State: StateStart}
	} else if c.TelegramUsername == "" && m.From.Username != "" {
		c.TelegramUsername = m.From.Username
	}
	// /start restarts the flow regardless of where we were.
	if strings.HasPrefix(strings.TrimSpace(m.Text), "/start") {
		c = &Conversation{TelegramID: m.From.ID, TelegramUsername: m.From.Username, State: StateStart}
		h.store.Set(c)
		h.askService(ctx, m.Chat.ID)
		return
	}

	switch c.State {
	case StateStart:
		// Anything else before /start: nudge the user.
		h.askService(ctx, m.Chat.ID)
		h.store.Set(c)
	case StateAskService:
		// Free-text service is accepted, but the user usually picks a button.
		c.Service = strings.TrimSpace(m.Text)
		c.State = StateAskProperty
		h.store.Set(c)
		h.askProperty(ctx, m.Chat.ID)
	case StateAskProperty:
		c.PropertyType = strings.TrimSpace(m.Text)
		c.State = StateAskBudget
		h.store.Set(c)
		h.askBudget(ctx, m.Chat.ID)
	case StateAskBudget:
		c.Budget = strings.TrimSpace(m.Text)
		c.State = StateAskLocation
		h.store.Set(c)
		h.askLocation(ctx, m.Chat.ID)
	case StateAskLocation:
		c.Location = strings.TrimSpace(m.Text)
		c.State = StateAskTimeline
		h.store.Set(c)
		h.askTimeline(ctx, m.Chat.ID)
	case StateAskTimeline:
		// Timeline usually arrives via callback; allow free text too.
		c.Timeline = strings.TrimSpace(m.Text)
		c.State = StateAskName
		h.store.Set(c)
		h.askName(ctx, m.Chat.ID)
	case StateAskName:
		c.Name = strings.TrimSpace(m.Text)
		c.State = StateAskPhone
		h.store.Set(c)
		h.askPhone(ctx, m.Chat.ID)
	case StateAskPhone:
		if m.Contact != nil {
			c.Phone = m.Contact.PhoneNumber
		} else {
			c.Phone = strings.TrimSpace(m.Text)
		}
		c.State = StateConfirm
		h.store.Set(c)
		h.askConfirm(ctx, m.Chat.ID, c)
	case StateConfirm, StateCompleted:
		// Ignore free text until they use the buttons.
		h.send(ctx, m.Chat.ID, "Please use the buttons above to confirm or cancel.", removeReplyKeyboard())
	}
}

// handleCallback processes a button press.
func (h *Handler) handleCallback(ctx context.Context, q *CallbackQuery) {
	if q.From == nil {
		return
	}
	c, ok := h.store.Get(q.From.ID)
	if !ok {
		// No active conversation — politely restart.
		_ = h.answerCallback(ctx, q.ID, "")
		_ = h.send(ctx, q.From.ID, "Please send /start to begin.", nil)
		return
	}
	parts := strings.SplitN(q.Data, ":", 2)
	if len(parts) != 2 {
		_ = h.answerCallback(ctx, q.ID, "")
		return
	}
	kind, val := parts[0], parts[1]
	_ = h.answerCallback(ctx, q.ID, "")

	switch kind {
	case "service":
		c.Service = serviceLabel(val)
		c.State = StateAskProperty
		h.store.Set(c)
		if q.Message != nil {
			h.askProperty(ctx, q.Message.Chat.ID)
		}
	case "prop":
		c.PropertyType = propertyLabel(val)
		c.State = StateAskBudget
		h.store.Set(c)
		if q.Message != nil {
			h.askBudget(ctx, q.Message.Chat.ID)
		}
	case "timeline":
		c.Timeline = timelineLabel(val)
		c.State = StateAskName
		h.store.Set(c)
		if q.Message != nil {
			h.askName(ctx, q.Message.Chat.ID)
		}
	case "confirm":
		switch val {
		case "yes":
			c.State = StateCompleted
			h.store.Set(c)
			if q.Message != nil {
				h.finalize(ctx, q.Message.Chat.ID, c)
			}
		case "edit":
			// Restart from service.
			c.State = StateAskService
			c.Service = ""
			c.PropertyType = ""
			c.Budget = ""
			c.Location = ""
			c.Timeline = ""
			c.Name = ""
			c.Phone = ""
			h.store.Set(c)
			if q.Message != nil {
				h.askService(ctx, q.Message.Chat.ID)
			}
		case "cancel":
			h.store.Delete(c.TelegramID)
			if q.Message != nil {
				_ = h.send(ctx, q.Message.Chat.ID, "Cancelled. Send /start to begin again.", nil)
			}
		}
	}
}

// Step messages -------------------------------------------------------------

func (h *Handler) askService(ctx context.Context, chatID int64) {
	_ = h.send(ctx, chatID, "Welcome! What are you looking for?", ServiceOptions())
}

func (h *Handler) askProperty(ctx context.Context, chatID int64) {
	_ = h.send(ctx, chatID, "What type of property?", PropertyTypeOptions())
}

func (h *Handler) askBudget(ctx context.Context, chatID int64) {
	_ = h.send(ctx, chatID,
		"What is your approximate budget? (e.g. ₹90L, 1.5 Cr, 5000000)",
		removeReplyKeyboard())
}

func (h *Handler) askLocation(ctx context.Context, chatID int64) {
	_ = h.send(ctx, chatID, "Which location are you interested in?", removeReplyKeyboard())
}

func (h *Handler) askTimeline(ctx context.Context, chatID int64) {
	_ = h.send(ctx, chatID, "When are you planning to make a decision?", TimelineOptions())
}

func (h *Handler) askName(ctx context.Context, chatID int64) {
	_ = h.send(ctx, chatID, "What is your name?", removeReplyKeyboard())
}

func (h *Handler) askPhone(ctx context.Context, chatID int64) {
	_ = h.send(ctx, chatID, "Please share your phone number.", ContactKeyboard())
}

func (h *Handler) askConfirm(ctx context.Context, chatID int64, c *Conversation) {
	text := "Please confirm your information:\n\n" +
		fmt.Sprintf("Service: %s\n", defaultStr(c.Service, "-")) +
		fmt.Sprintf("Property: %s\n", defaultStr(c.PropertyType, "-")) +
		fmt.Sprintf("Budget: %s\n", defaultStr(c.Budget, "-")) +
		fmt.Sprintf("Location: %s\n", defaultStr(c.Location, "-")) +
		fmt.Sprintf("Timeline: %s\n", defaultStr(c.Timeline, "-")) +
		fmt.Sprintf("Name: %s\n", defaultStr(c.Name, "-")) +
		fmt.Sprintf("Phone: %s", defaultStr(c.Phone, "-"))
	_ = h.send(ctx, chatID, text, ConfirmOptions())
}

// finalize creates the lead via the service, notifies the user, and clears state.
func (h *Handler) finalize(ctx context.Context, chatID int64, c *Conversation) {
	created, err := h.svc.CreateFromConversation(ctx, c.ToCreateInput())
	if err != nil {
		h.log.Warn("create lead failed", "err", err, "telegram_id", c.TelegramID)
		_ = h.send(ctx, chatID,
			"Sorry, something went wrong while saving your details. Please try again later.",
			nil)
		// Reset so they can retry.
		h.store.Delete(c.TelegramID)
		return
	}
	msg := fmt.Sprintf("Thank you! Your details have been received. Reference: %s", created.ID.Hex())
	_ = h.send(ctx, chatID, msg, nil)
	h.store.Delete(c.TelegramID)
}

func defaultStr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// ErrInvalidUpdate indicates a malformed update was rejected before processing.
var ErrInvalidUpdate = errors.New("invalid update")
