package str

import "testing"

func TestRandomStringLength(t *testing.T) {
	for _, n := range []int{0, 1, 8, 16, 32, 64, 128} {
		s := RandomString(n)
		if len(s) != n {
			t.Errorf("RandomString(%d) length = %d", n, len(s))
		}
	}
}

func TestRandomStringUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s := RandomString(16)
		if seen[s] {
			t.Fatalf("RandomString(16) produced duplicate: %q", s)
		}
		seen[s] = true
	}
}

func TestRandomStringCharset(t *testing.T) {
	s := RandomString(1000)
	allowed := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	allowedSet := make(map[byte]bool)
	for i := 0; i < len(allowed); i++ {
		allowedSet[allowed[i]] = true
	}
	for i := 0; i < len(s); i++ {
		if !allowedSet[s[i]] {
			t.Fatalf("RandomString contains invalid char: %c", s[i])
		}
	}
}
