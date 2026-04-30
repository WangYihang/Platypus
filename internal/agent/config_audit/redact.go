package config_audit

import "strings"

// RedactSecret masks a credential for transmission off the agent.
// Strings with at least 9 runes keep their first 4 and last 4 runes
// literal with a fixed-width "****" between them — enough context for
// an operator to spot the credential in an external system without
// reconstructing it. Shorter strings (or ones that may not even be
// secrets — short tokens, password placeholders) collapse to
// "********" so we never emit a literal value just because it happens
// to be 8 characters long.
//
// The function operates on runes so multi-byte secrets (UTF-8 in
// .env values, for instance) don't get sliced mid-codepoint.
func RedactSecret(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) < 9 {
		return "********"
	}
	var b strings.Builder
	b.Grow(4 + 4 + 4)
	b.WriteString(string(runes[:4]))
	b.WriteString("****")
	b.WriteString(string(runes[len(runes)-4:]))
	return b.String()
}
