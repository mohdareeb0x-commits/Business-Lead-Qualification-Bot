package tests

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/telegramleadbot/telegram-lead-bot/internal/bot"
	"github.com/telegramleadbot/telegram-lead-bot/internal/lead"
	"github.com/telegramleadbot/telegram-lead-bot/internal/scoring"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// stubRepo is an in-memory implementation of lead.Repository for unit tests.
type stubRepo struct {
	leads  map[primitive.ObjectID]*lead.Lead
	events []lead.LeadEvent
}

func newStubRepo() *stubRepo {
	return &stubRepo{leads: make(map[primitive.ObjectID]*lead.Lead)}
}

func (s *stubRepo) CreateLead(_ context.Context, l *lead.Lead) error {
	if l.ID.IsZero() {
		l.ID = primitive.NewObjectID()
	}
	cp := *l
	s.leads[l.ID] = &cp
	return nil
}

func (s *stubRepo) GetLeadByID(_ context.Context, id primitive.ObjectID) (*lead.Lead, error) {
	l, ok := s.leads[id]
	if !ok {
		return nil, lead.ErrNotFound
	}
	cp := *l
	return &cp, nil
}

func (s *stubRepo) ListLeads(_ context.Context, _ lead.ListLeadsQuery) ([]lead.Lead, int64, error) {
	out := make([]lead.Lead, 0, len(s.leads))
	for _, l := range s.leads {
		out = append(out, *l)
	}
	return out, int64(len(out)), nil
}

func (s *stubRepo) UpdateLeadStatus(_ context.Context, id primitive.ObjectID, status lead.Status) (*lead.Lead, error) {
	l, ok := s.leads[id]
	if !ok {
		return nil, lead.ErrNotFound
	}
	cp := *l
	cp.Status = status
	s.leads[id] = &cp
	return &cp, nil
}

func (s *stubRepo) CreateEvent(_ context.Context, e *lead.LeadEvent) error {
	if e.ID.IsZero() {
		e.ID = primitive.NewObjectID()
	}
	s.events = append(s.events, *e)
	return nil
}

func (s *stubRepo) ListEvents(_ context.Context, _ primitive.ObjectID, _, _ int) ([]lead.LeadEvent, int64, error) {
	return s.events, int64(len(s.events)), nil
}

func (s *stubRepo) DashboardStats(_ context.Context) (*lead.DashboardStats, error) {
	return &lead.DashboardStats{
		TotalLeads:        int64(len(s.leads)),
		ByStatus:          map[string]int64{},
		ByTemperature:     map[string]int64{},
		LeadsCreatedToday: 0,
	}, nil
}

// silentLogger discards everything so tests don't pollute stdout.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBotStateMachine_BeginToService(t *testing.T) {
	store := bot.NewMemoryStore()
	repo := newStubRepo()
	svc := lead.NewService(repo, scoring.New(), nil, silentLogger())
	h := bot.NewHandler("test-token", store, svc, silentLogger())

	h.HandleUpdate(context.Background(), bot.TelegramUpdate{
		UpdateID: 1,
		Message: &bot.TelegramMessage{
			From: &bot.User{ID: 7, Username: "alice"},
			Chat: bot.Chat{ID: 7},
			Text: "/start",
		},
	})
	// After /start, the conversation is stored as State=Start. The bot also
	// dispatches the first question immediately.
	c, ok := store.Get(7)
	if !ok {
		t.Fatal("expected conversation after /start")
	}
	if c.TelegramID != 7 {
		t.Errorf("telegram id = %d, want 7", c.TelegramID)
	}
}

func TestService_StatusTransitions(t *testing.T) {
	repo := newStubRepo()
	svc := lead.NewService(repo, scoring.New(), nil, silentLogger())

	created, err := svc.CreateFromConversation(context.Background(), lead.CreateLeadInput{
		TelegramID:   7,
		Name:         "Ahmed",
		Phone:        "+911234567890",
		Service:      "Buy Property",
		Requirements: lead.Requirements{PropertyType: "Apartment"},
		Budget:       "10000000",
		Location:     "Noida",
		Timeline:     "Immediately",
	})
	if err != nil {
		t.Fatalf("CreateFromConversation: %v", err)
	}
	if created.Status != lead.StatusNew {
		t.Errorf("status = %q, want NEW", created.Status)
	}
	if created.Score < 80 {
		t.Errorf("expected HOT score, got %d", created.Score)
	}

	for _, next := range []lead.Status{lead.StatusContacted, lead.StatusQualified, lead.StatusConverted} {
		updated, err := svc.UpdateStatus(context.Background(), created.ID, next)
		if err != nil {
			t.Fatalf("UpdateStatus(%s): %v", next, err)
		}
		if updated.Status != next {
			t.Errorf("status = %q, want %q", updated.Status, next)
		}
	}

	if _, err := svc.UpdateStatus(context.Background(), created.ID, lead.Status("NOPE")); err == nil {
		t.Error("expected error on invalid status")
	}

	// 2 events on create (LEAD_CREATED + LEAD_SCORED) + 3 transitions.
	if got := len(repo.events); got < 5 {
		t.Errorf("expected >= 5 events, got %d", got)
	}
}

func TestService_RejectsInvalidInput(t *testing.T) {
	repo := newStubRepo()
	svc := lead.NewService(repo, scoring.New(), nil, silentLogger())
	_, err := svc.CreateFromConversation(context.Background(), lead.CreateLeadInput{
		Name:     "",
		Phone:    "+911234567890",
		Service:  "Buy Property",
		Timeline: "Immediately",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestService_ValidatesInvalidStatus(t *testing.T) {
	repo := newStubRepo()
	svc := lead.NewService(repo, scoring.New(), nil, silentLogger())
	created, err := svc.CreateFromConversation(context.Background(), lead.CreateLeadInput{
		Name:     "Ahmed",
		Phone:    "+911234567890",
		Service:  "Buy Property",
		Timeline: "Immediately",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.UpdateStatus(context.Background(), created.ID, "bogus"); err == nil {
		t.Fatal("expected error on invalid status")
	}
}
