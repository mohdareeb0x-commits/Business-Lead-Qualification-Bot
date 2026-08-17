package lead

import (
	"errors"
	"strings"
)

// Pagination params shared by list endpoints.
const (
	DefaultPage  = 1
	DefaultLimit = 20
	MaxLimit     = 100
)

// ListLeadsQuery holds validated query params for GET /api/v1/leads.
type ListLeadsQuery struct {
	Page        int
	Limit       int
	Status      string
	Temperature string
}

// Pagination is the metadata block returned with paginated lists.
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// ListLeadsResponse is the wire format of GET /api/v1/leads.
type ListLeadsResponse struct {
	Data       []Lead     `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// ListEventsResponse is the wire format of GET /api/v1/leads/:id/events.
type ListEventsResponse struct {
	Data       []LeadEvent `json:"data"`
	Pagination Pagination  `json:"pagination"`
}

// UpdateStatusRequest is the body of PATCH /api/v1/leads/:id/status.
type UpdateStatusRequest struct {
	Status Status `json:"status"`
}

// DashboardStats is the wire format of GET /api/v1/dashboard/stats.
type DashboardStats struct {
	TotalLeads        int64            `json:"total_leads"`
	ByStatus          map[string]int64 `json:"by_status"`
	ByTemperature     map[string]int64 `json:"by_temperature"`
	LeadsCreatedToday int64            `json:"leads_created_today"`
}

// Validate enforces the constraints documented in the spec.
func (q *ListLeadsQuery) Validate() error {
	if q.Page <= 0 {
		return errors.New("page must be >= 1")
	}
	if q.Limit <= 0 {
		return errors.New("limit must be >= 1")
	}
	if q.Limit > MaxLimit {
		return errors.New("limit exceeds maximum")
	}
	if q.Status != "" && !IsValidStatus(Status(strings.ToUpper(q.Status))) {
		return errors.New("invalid status filter")
	}
	if q.Temperature != "" {
		t := Temperature(strings.ToUpper(q.Temperature))
		ok := false
		for _, v := range ValidTemperatures {
			if t == v {
				ok = true
				break
			}
		}
		if !ok {
			return errors.New("invalid temperature filter")
		}
	}
	return nil
}

// Normalize upper-cases the status and temperature filter values.
func (q *ListLeadsQuery) Normalize() {
	q.Status = strings.ToUpper(q.Status)
	q.Temperature = strings.ToUpper(q.Temperature)
}
