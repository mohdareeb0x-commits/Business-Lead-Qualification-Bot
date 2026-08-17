package lead

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/telegramleadbot/telegram-lead-bot/internal/scoring"
)

// Notifier is the small surface Service needs to inform the admin of a new
// lead. Implemented by internal/notification.Telegram.
type Notifier interface {
	NotifyNewLead(ctx context.Context, l *Lead) error
}

// Scorer computes a deterministic score and temperature from a Lead draft.
type Scorer interface {
	Score(in scoring.Input) scoring.Result
}

// Service holds the business logic for leads. Handlers and the Telegram bot
// depend on this — they never touch the repository directly.
type Service struct {
	repo   Repository
	scorer Scorer
	notif  Notifier
	log    *slog.Logger
}

// NewService wires dependencies.
func NewService(repo Repository, scorer Scorer, notif Notifier, log *slog.Logger) *Service {
	return &Service{repo: repo, scorer: scorer, notif: notif, log: log}
}

// CreateLeadInput is the validated input the bot hands to the service.
type CreateLeadInput struct {
	TelegramID       int64
	TelegramUsername string
	Name             string
	Phone            string
	Service          string
	Requirements     Requirements
	Budget           string
	Location         string
	Timeline         string
}

// ErrInvalidInput is returned for any validation failure the service catches.
var ErrInvalidInput = errors.New("invalid input")

// CreateFromConversation validates a bot submission, scores it, persists the
// lead, records the initial events, and notifies the admin. It returns the
// saved lead or an error suitable for surfacing to the user.
func (s *Service) CreateFromConversation(ctx context.Context, in CreateLeadInput) (*Lead, error) {
	if err := ValidateCreateInput(in); err != nil {
		return nil, err
	}

	lead := &Lead{
		TelegramID:       in.TelegramID,
		TelegramUsername: strings.TrimSpace(in.TelegramUsername),
		Name:             strings.TrimSpace(in.Name),
		Phone:            NormalizePhone(in.Phone),
		Service:          in.Service,
		Requirements:     in.Requirements,
		Budget:           strings.TrimSpace(in.Budget),
		Location:         strings.TrimSpace(in.Location),
		Timeline:         strings.TrimSpace(in.Timeline),
		Source:           SourceTelegram,
	}
	res := s.scorer.Score(scoring.Input{
		Phone:    lead.Phone,
		Budget:   lead.Budget,
		Location: lead.Location,
		Timeline: lead.Timeline,
	})
	lead.Score = res.Score
	lead.Temperature = Temperature(res.Temperature)
	lead.Status = StatusNew

	if err := s.repo.CreateLead(ctx, lead); err != nil {
		return nil, err
	}

	// LEAD_CREATED event with the initial score snapshot.
	meta := map[string]interface{}{
		"score":       lead.Score,
		"temperature": string(lead.Temperature),
		"source":      string(lead.Source),
	}
	if err := s.repo.CreateEvent(ctx, &LeadEvent{
		LeadID:    lead.ID,
		EventType: EventLeadCreated,
		Metadata:  meta,
	}); err != nil {
		s.log.Warn("create LEAD_CREATED event failed", "err", err)
	}
	if err := s.repo.CreateEvent(ctx, &LeadEvent{
		LeadID:    lead.ID,
		EventType: EventLeadScored,
		Metadata:  map[string]interface{}{"score": lead.Score, "temperature": string(lead.Temperature)},
	}); err != nil {
		s.log.Warn("create LEAD_SCORED event failed", "err", err)
	}

	if s.notif != nil {
		if err := s.notif.NotifyNewLead(ctx, lead); err != nil {
			// Notification failure must not lose the lead; log and move on.
			s.log.Warn("admin notification failed", "err", err, "lead_id", lead.ID.Hex())
		}
	}
	return lead, nil
}

// Get returns a lead by ID or ErrNotFound.
func (s *Service) Get(ctx context.Context, id primitive.ObjectID) (*Lead, error) {
	return s.repo.GetLeadByID(ctx, id)
}

// List returns paginated, optionally-filtered leads.
func (s *Service) List(ctx context.Context, q ListLeadsQuery) (*ListLeadsResponse, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	q.Normalize()
	leads, total, err := s.repo.ListLeads(ctx, q)
	if err != nil {
		return nil, err
	}
	if leads == nil {
		leads = []Lead{}
	}
	totalPages := int((total + int64(q.Limit) - 1) / int64(q.Limit))
	if totalPages == 0 {
		totalPages = 1
	}
	return &ListLeadsResponse{
		Data: leads,
		Pagination: Pagination{
			Page:       q.Page,
			Limit:      q.Limit,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

// UpdateStatus changes a lead's status, records a STATUS_CHANGED event with
// the previous and new status in metadata, and returns the updated lead.
func (s *Service) UpdateStatus(ctx context.Context, id primitive.ObjectID, next Status) (*Lead, error) {
	if !IsValidStatus(next) {
		return nil, ErrInvalidInput
	}
	current, err := s.repo.GetLeadByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.Status == next {
		// Idempotent — return as-is, don't record a duplicate event.
		return current, nil
	}
	updated, err := s.repo.UpdateLeadStatus(ctx, id, next)
	if err != nil {
		return nil, err
	}
	meta := map[string]interface{}{
		"from": string(current.Status),
		"to":   string(next),
	}
	if err := s.repo.CreateEvent(ctx, &LeadEvent{
		LeadID:    id,
		EventType: EventStatusChanged,
		Metadata:  meta,
	}); err != nil {
		s.log.Warn("create STATUS_CHANGED event failed", "err", err)
	}
	return updated, nil
}

// ListEvents returns paginated events for a lead (ascending by time).
func (s *Service) ListEvents(ctx context.Context, leadID primitive.ObjectID, page, limit int) (*ListEventsResponse, error) {
	if page <= 0 {
		page = DefaultPage
	}
	if limit <= 0 || limit > MaxLimit {
		limit = DefaultLimit
	}
	events, total, err := s.repo.ListEvents(ctx, leadID, page, limit)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []LeadEvent{}
	}
	totalPages := int((total + int64(limit) - 1) / int64(limit))
	if totalPages == 0 {
		totalPages = 1
	}
	return &ListEventsResponse{
		Data: events,
		Pagination: Pagination{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

// Stats exposes the dashboard aggregation.
func (s *Service) Stats(ctx context.Context) (*DashboardStats, error) {
	return s.repo.DashboardStats(ctx)
}

// ValidateCreateInput checks the bot submission. Returns ErrInvalidInput on
// any failure with a wrapped message describing the offending field.
func ValidateCreateInput(in CreateLeadInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return wrap("name is required")
	}
	if !ValidPhone(in.Phone) {
		return wrap("phone is required and must be valid")
	}
	if strings.TrimSpace(in.Service) == "" {
		return wrap("service is required")
	}
	if strings.TrimSpace(in.Timeline) == "" {
		return wrap("timeline is required")
	}
	return nil
}

func wrap(msg string) error {
	return errors.Join(ErrInvalidInput, errors.New(msg))
}
