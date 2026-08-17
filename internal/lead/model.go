package lead

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Temperature classifies a lead's buying intent.
type Temperature string

const (
	TemperatureHot  Temperature = "HOT"
	TemperatureWarm Temperature = "WARM"
	TemperatureCold Temperature = "COLD"
)

// ValidTemperatures is the closed set used for validation.
var ValidTemperatures = []Temperature{
	TemperatureHot, TemperatureWarm, TemperatureCold,
}

// Status is the lifecycle stage of a lead.
type Status string

const (
	StatusNew       Status = "NEW"
	StatusContacted Status = "CONTACTED"
	StatusQualified Status = "QUALIFIED"
	StatusConverted Status = "CONVERTED"
	StatusLost      Status = "LOST"
)

// ValidStatuses is the closed set used for validation and PATCH /status.
var ValidStatuses = []Status{
	StatusNew, StatusContacted, StatusQualified, StatusConverted, StatusLost,
}

// IsValidStatus reports whether s is a known status.
func IsValidStatus(s Status) bool {
	for _, v := range ValidStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Source identifies the channel that produced a lead.
type Source string

const (
	SourceTelegram Source = "TELEGRAM"
)

// Lead is the canonical lead record stored in MongoDB.
type Lead struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TelegramID       int64              `bson:"telegram_id" json:"telegram_id"`
	TelegramUsername string             `bson:"telegram_username,omitempty" json:"telegram_username,omitempty"`
	Name             string             `bson:"name" json:"name"`
	Phone            string             `bson:"phone" json:"phone"`
	Service          string             `bson:"service" json:"service"`
	Requirements     Requirements       `bson:"requirements" json:"requirements"`
	Budget           string             `bson:"budget,omitempty" json:"budget,omitempty"`
	Location         string             `bson:"location,omitempty" json:"location,omitempty"`
	Timeline         string             `bson:"timeline,omitempty" json:"timeline,omitempty"`
	Score            int                `bson:"score" json:"score"`
	Temperature      Temperature        `bson:"temperature" json:"temperature"`
	Status           Status             `bson:"status" json:"status"`
	Source           Source             `bson:"source" json:"source"`
	CreatedAt        time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time          `bson:"updated_at" json:"updated_at"`
}

// Requirements holds the questionnaire answers that vary by industry.
// For the real-estate MVP it captures property type and free-form notes.
type Requirements struct {
	PropertyType string `bson:"property_type,omitempty" json:"property_type,omitempty"`
	Notes        string `bson:"notes,omitempty" json:"notes,omitempty"`
}

// EventType discriminates entries in the lead_events collection.
type EventType string

const (
	EventLeadCreated    EventType = "LEAD_CREATED"
	EventLeadScored     EventType = "LEAD_SCORED"
	EventStatusChanged  EventType = "STATUS_CHANGED"
	EventContactUpdated EventType = "CONTACT_UPDATED"
)

// LeadEvent is a single immutable audit record for a lead.
type LeadEvent struct {
	ID        primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	LeadID    primitive.ObjectID     `bson:"lead_id" json:"lead_id"`
	EventType EventType              `bson:"event_type" json:"event_type"`
	Metadata  map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`
	CreatedAt time.Time              `bson:"created_at" json:"created_at"`
}
