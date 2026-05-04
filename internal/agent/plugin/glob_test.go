package plugin

import "testing"

func TestMatchGlob_NoMeta(t *testing.T) {
	cases := []struct {
		p, v string
		want bool
	}{
		{"/bin/ls", "/bin/ls", true},
		{"/bin/ls", "/bin/cat", false},
		{"", "", true},
		{"foo", "", false},
		{"", "foo", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.p, c.v, 0); got != c.want {
			t.Errorf("matchGlob(%q, %q, 0) = %v; want %v", c.p, c.v, got, c.want)
		}
	}
}

func TestMatchGlob_SingleStar(t *testing.T) {
	// pathSep=0: * matches anything including separators.
	cases := []struct {
		p, v string
		want bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"prefix*", "prefix", true},
		{"prefix*", "prefixsuffix", true},
		{"prefix*", "noprefix", false},
		{"*suffix", "anysuffix", true},
		{"*suffix", "suffix", true},
		{"*suffix", "wrong", false},
		{"a*b", "ab", true},
		{"a*b", "axb", true},
		{"a*b", "axyzb", true},
		{"a*b", "axyz", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.p, c.v, 0); got != c.want {
			t.Errorf("matchGlob(%q, %q, 0) = %v; want %v", c.p, c.v, got, c.want)
		}
	}
}

func TestMatchGlob_PathSeparator(t *testing.T) {
	// pathSep='/': * stops at /, ** crosses it.
	cases := []struct {
		p, v string
		want bool
	}{
		{"/etc/*", "/etc/passwd", true},
		{"/etc/*", "/etc/nginx/conf", false}, // single * doesn't cross /
		{"/etc/**", "/etc/nginx/conf", true},
		{"/etc/**", "/etc", true}, // ** matches empty too
		{"/etc/**", "/etc/", true},
		{"/var/log/*.log", "/var/log/foo.log", true},
		{"/var/log/*.log", "/var/log/sub/foo.log", false},
		{"/var/log/**/*.log", "/var/log/sub/foo.log", true},
		{"/var/log/**/*.log", "/var/log/foo.log", true}, // ** matches empty
		{"/etc/*.conf", "/etc/.conf", true},             // empty * still matches
		{"/etc/*.conf", "/etc/foo.conf", true},
		{"/etc/*.conf", "/etc/foo.conf.bak", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.p, c.v, '/'); got != c.want {
			t.Errorf("matchGlob(%q, %q, '/') = %v; want %v", c.p, c.v, got, c.want)
		}
	}
}

func TestMatchGlob_Question(t *testing.T) {
	cases := []struct {
		p, v string
		sep  byte
		want bool
	}{
		{"?", "a", 0, true},
		{"?", "ab", 0, false},
		{"?", "", 0, false},
		{"a?c", "abc", 0, true},
		{"a?c", "ac", 0, false},
		{"/etc/host?", "/etc/hosts", '/', true},
		{"/etc/host?", "/etc/host", '/', false},
		{"/?/x", "/a/x", '/', true},
		{"/?/x", "//x", '/', false},
	}
	for _, c := range cases {
		if got := matchGlob(c.p, c.v, c.sep); got != c.want {
			t.Errorf("matchGlob(%q, %q, %q) = %v; want %v", c.p, c.v, c.sep, got, c.want)
		}
	}
}

func TestMatchGlob_DoubleStar(t *testing.T) {
	cases := []struct {
		p, v string
		want bool
	}{
		{"**", "", true},
		{"**", "/anything/here", true},
		{"/foo/**/bar", "/foo/bar", true},
		{"/foo/**/bar", "/foo/x/bar", true},
		{"/foo/**/bar", "/foo/x/y/bar", true},
		{"/foo/**/bar", "/foo/bar/x", false},
		{"**/x", "x", true},
		{"**/x", "/a/b/x", true},
		{"**/x", "/a/b/x/y", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.p, c.v, '/'); got != c.want {
			t.Errorf("matchGlob(%q, %q, '/') = %v; want %v", c.p, c.v, got, c.want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{"/bin/ls", "/usr/bin/*"}
	cases := []struct {
		v    string
		want bool
	}{
		{"/bin/ls", true},
		{"/usr/bin/cat", true},
		{"/usr/bin/sub/cat", false}, // * doesn't cross /
		{"/bin/cat", false},
	}
	for _, c := range cases {
		if got := matchAny(patterns, c.v, '/'); got != c.want {
			t.Errorf("matchAny(%q) = %v; want %v", c.v, got, c.want)
		}
	}
	// Star fast path: ["*"] matches anything.
	if !matchAny([]string{"*"}, "/anything/here", '/') {
		t.Errorf(`["*"] should match anything`)
	}
}
