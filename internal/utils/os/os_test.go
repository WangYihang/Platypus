package os

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		input    string
		expected OperatingSystem
	}{
		{"linux", Linux},
		{"windows", Windows},
		{"darwin", MacOS},
		{"freebsd", FreeBSD},
		{"unknown-os", Unknown},
		{"", Unknown},
		{"Linux", Unknown}, // case-sensitive
	}
	for _, tt := range tests {
		got := Parse(tt.input)
		if got != tt.expected {
			t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestString(t *testing.T) {
	if Linux.String() == "" {
		t.Error("Linux.String() should not be empty")
	}
	if Unknown.String() != "Unknown" {
		t.Errorf("Unknown.String() = %q, want %q", Unknown.String(), "Unknown")
	}
}
