// Package bot contains the Telegram bot: state machine, keyboard layouts,
// webhook handler, and outbound message helpers.
package bot

import (
	"errors"
	"sync"
	"time"

	"github.com/telegramleadbot/telegram-lead-bot/internal/lead"
)

// State is one step in the lead-qualification conversation.
type State string

const (
	StateStart       State = "START"
	StateAskService  State = "ASK_SERVICE"
	StateAskProperty State = "ASK_PROPERTY_TYPE"
	StateAskBudget   State = "ASK_BUDGET"
	StateAskLocation State = "ASK_LOCATION"
	StateAskTimeline State = "ASK_TIMELINE"
	StateAskName     State = "ASK_NAME"
	StateAskPhone    State = "ASK_PHONE"
	StateConfirm     State = "CONFIRM"
	StateCompleted   State = "COMPLETED"
)

// Conversation is the per-user draft the bot accumulates before persisting.
type Conversation struct {
	TelegramID       int64
	TelegramUsername string
	State            State
	Service          string
	PropertyType     string
	Budget           string
	Location         string
	Timeline         string
	Name             string
	Phone            string
	UpdatedAt        time.Time
}

// ToCreateInput snapshots the conversation into the lead service's input.
func (c *Conversation) ToCreateInput() lead.CreateLeadInput {
	return lead.CreateLeadInput{
		TelegramID:       c.TelegramID,
		TelegramUsername: c.TelegramUsername,
		Name:             c.Name,
		Phone:            c.Phone,
		Service:          c.Service,
		Requirements:     lead.Requirements{PropertyType: c.PropertyType},
		Budget:           c.Budget,
		Location:         c.Location,
		Timeline:         c.Timeline,
	}
}

// Store is the conversation-state interface. The MVP uses an in-memory
// implementation; swapping in Redis later requires only a new implementation
// of this interface.
type Store interface {
	Get(telegramID int64) (*Conversation, bool)
	Set(c *Conversation)
	Delete(telegramID int64)
}

// ErrNotFound indicates a conversation does not exist for the given id.
var ErrNotFound = errors.New("conversation not found")

// expiry is how long an idle conversation lives before being discarded.
const expiry = 30 * time.Minute

// memoryStore is a goroutine-safe in-memory Store with TTL eviction on read.
type memoryStore struct {
	mu   sync.Mutex
	data map[int64]*Conversation
	now  func() time.Time
}

// NewMemoryStore returns a Store backed by an in-process map.
func NewMemoryStore() Store {
	return &memoryStore{
		data: make(map[int64]*Conversation),
		now:  time.Now,
	}
}

func (m *memoryStore) Get(id int64) (*Conversation, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.data[id]
	if !ok {
		return nil, false
	}
	if m.now().Sub(c.UpdatedAt) > expiry {
		delete(m.data, id)
		return nil, false
	}
	return c, true
}

func (m *memoryStore) Set(c *Conversation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.UpdatedAt = m.now()
	m.data[c.TelegramID] = c
}

func (m *memoryStore) Delete(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, id)
}
