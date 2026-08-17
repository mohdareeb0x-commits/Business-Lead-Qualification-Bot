package tests

import (
	"errors"
	"testing"

	"github.com/telegramleadbot/telegram-lead-bot/internal/lead"
)

func TestValidPhone(t *testing.T) {
	good := []string{"+911234567890", "9876543210", "+1 555 123 4567", "555-123-4567"}
	bad := []string{"", "abc", "12", "+", "+++++++++++++++++"}
	for _, p := range good {
		if !lead.ValidPhone(p) {
			t.Errorf("expected %q to be valid", p)
		}
	}
	for _, p := range bad {
		if lead.ValidPhone(p) {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"+91 123 456 7890": "+911234567890",
		"555-123-4567":     "5551234567",
		"  +91 1234 ":      "+911234",
	}
	for in, want := range cases {
		if got := lead.NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsValidStatus(t *testing.T) {
	valid := []lead.Status{lead.StatusNew, lead.StatusContacted, lead.StatusQualified, lead.StatusConverted, lead.StatusLost}
	invalid := []lead.Status{"", "WAT", "new "}
	for _, s := range valid {
		if !lead.IsValidStatus(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range invalid {
		if lead.IsValidStatus(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestListLeadsQueryValidate(t *testing.T) {
	tests := []struct {
		name    string
		q       lead.ListLeadsQuery
		wantErr bool
	}{
		{"defaults ok", lead.ListLeadsQuery{Page: 1, Limit: 20}, false},
		{"page zero", lead.ListLeadsQuery{Page: 0, Limit: 20}, true},
		{"limit zero", lead.ListLeadsQuery{Page: 1, Limit: 0}, true},
		{"limit too large", lead.ListLeadsQuery{Page: 1, Limit: 9999}, true},
		{"bad status", lead.ListLeadsQuery{Page: 1, Limit: 20, Status: "bogus"}, true},
		{"good status", lead.ListLeadsQuery{Page: 1, Limit: 20, Status: "new"}, false},
		{"bad temp", lead.ListLeadsQuery{Page: 1, Limit: 20, Temperature: "lukewarm"}, true},
		{"good temp", lead.ListLeadsQuery{Page: 1, Limit: 20, Temperature: "hot"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.q.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateCreateInput(t *testing.T) {
	base := lead.CreateLeadInput{
		Name:     "Ahmed",
		Phone:    "+911234567890",
		Service:  "Buy Property",
		Timeline: "Immediately",
	}
	if err := lead.ValidateCreateInput(base); err != nil {
		t.Fatalf("baseline should be valid: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*lead.CreateLeadInput)
	}{
		{"empty name", func(i *lead.CreateLeadInput) { i.Name = "" }},
		{"empty phone", func(i *lead.CreateLeadInput) { i.Phone = "" }},
		{"bad phone", func(i *lead.CreateLeadInput) { i.Phone = "abc" }},
		{"empty service", func(i *lead.CreateLeadInput) { i.Service = "" }},
		{"empty timeline", func(i *lead.CreateLeadInput) { i.Timeline = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mut(&in)
			err := lead.ValidateCreateInput(in)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !errors.Is(err, lead.ErrInvalidInput) {
				t.Errorf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}
