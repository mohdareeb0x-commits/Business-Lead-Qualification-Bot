// Package scoring implements deterministic, rule-based lead scoring.
// Rules are isolated here so the Telegram bot and the repository never need
// to know how a score is calculated. To change scoring rules for a new
// industry, edit this file or swap in a different implementation.
//
// The package is intentionally decoupled from the lead package to avoid
// import cycles: it operates on a small Input and returns a Result.
package scoring

import "strings"

// Temperature classifies a lead's buying intent.
type Temperature string

const (
	Hot  Temperature = "HOT"
	Warm Temperature = "WARM"
	Cold Temperature = "COLD"
)

// Input is the small set of fields the scorer needs.
type Input struct {
	Phone    string
	Budget   string
	Location string
	Timeline string
}

// Result is what the scorer produces.
type Result struct {
	Score       int
	Temperature Temperature
}

// Scorer is the contract used by the lead service.
type Scorer struct{}

// New returns a Scorer with the default rule set.
func New() *Scorer { return &Scorer{} }

// Score applies the rule set and returns the result.
func (s *Scorer) Score(in Input) Result {
	score := 0
	score += scoreBudget(parseBudgetNumber(in.Budget))
	score += scoreTimeline(in.Timeline)
	score += scoreLocation(in.Location)
	if validPhone(in.Phone) {
		score += 10
	}
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return Result{Score: score, Temperature: classify(score)}
}

func parseBudgetNumber(s string) int64 {
	digits := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return 0
	}
	var n int64
	for _, r := range digits {
		n = n*10 + int64(r-'0')
	}
	return n
}

func scoreBudget(amount int64) int {
	switch {
	case amount >= 10_000_000:
		return 30
	case amount >= 5_000_000:
		return 20
	default:
		return 10
	}
}

func scoreTimeline(t string) int {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "immediately":
		return 30
	case "1-3 months", "1-3 month":
		return 20
	case "3-6 months", "3-6 month":
		return 10
	case "researching", "just researching":
		return 5
	default:
		return 0
	}
}

func scoreLocation(loc string) int {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return 0
	}
	return 20
}

func validPhone(p string) bool {
	if len(p) < 7 || len(p) > 20 {
		return false
	}
	if p[0] == '+' {
		p = p[1:]
	}
	for _, r := range p {
		if r == ' ' || r == '-' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func classify(score int) Temperature {
	switch {
	case score >= 80:
		return Hot
	case score >= 50:
		return Warm
	default:
		return Cold
	}
}
