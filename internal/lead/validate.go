package lead

import "regexp"

// phoneRegex is intentionally permissive: most countries allow 7–15 digits,
// optional leading + and dashes/spaces for readability.
var phoneRegex = regexp.MustCompile(`^\+?[0-9 \-]{7,20}$`)

// ValidPhone reports whether p looks like a real phone number.
func ValidPhone(p string) bool {
	return phoneRegex.MatchString(p)
}

// NormalizePhone strips spaces and dashes but preserves the leading +.
func NormalizePhone(p string) string {
	out := make([]rune, 0, len(p))
	for _, r := range p {
		if r == ' ' || r == '-' {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
