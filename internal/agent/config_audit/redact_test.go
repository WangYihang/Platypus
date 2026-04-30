package config_audit

import "testing"

func TestRedactSecret(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"a", "********"},
		{"12345678", "********"},
		{"123456789", "1234****6789"},
		{"AKIAIOSFODNN7EXAMPLE", "AKIA****MPLE"},
		// Multi-byte runes must not be sliced mid-codepoint.
		{"αβγδεζηθι", "αβγδ****ζηθι"},
	}
	for _, c := range cases {
		got := RedactSecret(c.in)
		if got != c.want {
			t.Errorf("RedactSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
