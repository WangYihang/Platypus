package plugin

import "testing"

func TestValidKVKey(t *testing.T) {
	cases := []struct {
		key string
		ok  bool
	}{
		{"", false},
		{".", false},
		{"..", false},
		{".hidden", false},
		{"plain", true},
		{"a-b_c.d", true},
		{"with/slash", false},
		{"a b", false},
		{"a\x00b", false},
		{"abc/../etc/passwd", false},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			if got := validKVKey(tc.key); got != tc.ok {
				t.Errorf("validKVKey(%q) = %v, want %v", tc.key, got, tc.ok)
			}
		})
	}
	// 129-char key — over the cap.
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	if validKVKey(string(long)) {
		t.Errorf("expected over-long key to be rejected")
	}
}
