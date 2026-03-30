package hash

import "testing"

func TestMD5(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"hello", "5d41402abc4b2a76b9719d911017c592"},
		{"platypus", "d293c98482fd37cff714ee96610174d6"},
	}
	for _, tt := range tests {
		got := MD5(tt.input)
		if got != tt.expected {
			t.Errorf("MD5(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMD5Deterministic(t *testing.T) {
	a := MD5("test")
	b := MD5("test")
	if a != b {
		t.Errorf("MD5 not deterministic: %q != %q", a, b)
	}
}

func TestMD5Length(t *testing.T) {
	got := MD5("any input")
	if len(got) != 32 {
		t.Errorf("MD5 hash length = %d, want 32", len(got))
	}
}
