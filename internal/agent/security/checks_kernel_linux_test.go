//go:build linux

package security

import "testing"

func TestParseKernelVersion(t *testing.T) {
	cases := []struct {
		in                  string
		major, minor, patch int
		ok                  bool
	}{
		{"5.15.0-101-generic", 5, 15, 0, true},
		{"6.1.55+", 6, 1, 55, true},
		{"4.19.123", 4, 19, 123, true},
		{"3.10.0-1160.el7.x86_64", 3, 10, 0, true},
		{"6.7", 6, 7, 0, true},
		{"6.10.0rc1", 6, 10, 0, true},
		{"garbage", 0, 0, 0, false},
		{"", 0, 0, 0, false},
	}
	for _, c := range cases {
		maj, min, pat, ok := parseKernelVersion(c.in)
		if ok != c.ok || maj != c.major || min != c.minor || pat != c.patch {
			t.Errorf("parseKernelVersion(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				c.in, maj, min, pat, ok, c.major, c.minor, c.patch, c.ok)
		}
	}
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b [3]int
		want int // sign(want) is the only thing we assert
	}{
		{[3]int{5, 15, 0}, [3]int{5, 4, 0}, +1},
		{[3]int{5, 4, 0}, [3]int{5, 15, 0}, -1},
		{[3]int{6, 1, 0}, [3]int{6, 1, 0}, 0},
		{[3]int{6, 1, 5}, [3]int{6, 1, 9}, -1},
		{[3]int{4, 19, 0}, [3]int{5, 0, 0}, -1},
	}
	for _, c := range cases {
		got := compareVersion(c.a[0], c.a[1], c.a[2], c.b[0], c.b[1], c.b[2])
		if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) || (got == 0) != (c.want == 0) {
			t.Errorf("compareVersion(%v,%v) sign mismatch: got %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}
