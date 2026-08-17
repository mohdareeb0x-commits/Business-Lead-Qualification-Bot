package tests

import (
	"testing"

	"github.com/telegramleadbot/telegram-lead-bot/internal/scoring"
)

func TestScore_Budget(t *testing.T) {
	s := scoring.New()
	cases := []struct {
		budget string
		min    int
	}{
		{"", 10},         // no digits → default bucket
		{"₹90L", 10},     // 90 < 5M
		{"5000000", 20},  // exactly 5M
		{"7500000", 20},  // 7.5M
		{"10000000", 30}, // 10M
		{"50 Cr", 10},    // digit run = 50 < 5M
	}
	for _, c := range cases {
		got := s.Score(scoring.Input{Budget: c.budget})
		if got.Score < c.min {
			t.Errorf("budget %q: got %d, want >= %d", c.budget, got.Score, c.min)
		}
	}
}

func TestScore_Timeline(t *testing.T) {
	s := scoring.New()
	cases := []struct {
		timeline string
		min      int
	}{
		{"immediately", 30},
		{"1-3 months", 20},
		{"3-6 months", 10},
		{"Just researching", 5},
		{"", 0},
	}
	for _, c := range cases {
		got := s.Score(scoring.Input{Timeline: c.timeline})
		if got.Score < c.min {
			t.Errorf("timeline %q: got %d, want >= %d", c.timeline, got.Score, c.min)
		}
	}
}

func TestScore_Clamping(t *testing.T) {
	s := scoring.New()
	r := s.Score(scoring.Input{
		Phone:    "+911234567890",
		Budget:   "10000000",
		Location: "Noida",
		Timeline: "immediately",
	})
	// 10 (phone) + 30 (10M) + 20 (loc) + 30 (immediately) = 90
	if r.Score != 90 {
		t.Errorf("expected score 90, got %d", r.Score)
	}
	if r.Score > 100 {
		t.Errorf("score should never exceed 100, got %d", r.Score)
	}
	if r.Temperature != scoring.Hot {
		t.Errorf("expected HOT, got %s", r.Temperature)
	}
}

func TestScore_TemperatureBands(t *testing.T) {
	s := scoring.New()
	cases := []struct {
		name string
		in   scoring.Input
		want scoring.Temperature
	}{
		{
			name: "hot — high everything",
			in:   scoring.Input{Phone: "+911234567890", Budget: "10000000", Location: "Noida", Timeline: "immediately"},
			want: scoring.Hot,
		},
		{
			name: "warm — mid budget + 1-3 months",
			in:   scoring.Input{Phone: "+911234567890", Budget: "5000000", Location: "Noida", Timeline: "1-3 months"},
			want: scoring.Warm,
		},
		{
			name: "cold — only baseline + researching",
			in:   scoring.Input{Phone: "", Budget: "1000", Location: "", Timeline: "researching"},
			want: scoring.Cold,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.Score(tc.in)
			if got.Temperature != tc.want {
				t.Errorf("temperature = %s, want %s (score=%d)", got.Temperature, tc.want, got.Score)
			}
		})
	}
}
