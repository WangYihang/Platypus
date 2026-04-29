package ipinfo

import "testing"

func TestLookupClassification(t *testing.T) {
	cases := []struct {
		name        string
		ip          string
		wantVersion int
		wantPrivate bool
		wantLoop    bool
	}{
		{"loopback v4", "127.0.0.1", 4, false, true},
		{"rfc1918 ten-net", "10.0.0.1", 4, true, false},
		{"rfc1918 192.168", "192.168.1.5", 4, true, false},
		{"cgnat", "100.64.0.1", 4, true, false},
		{"link-local v4", "169.254.10.10", 4, true, false},
		{"loopback v6", "::1", 6, false, true},
		{"unspecified v6", "::", 6, true, false},
		{"global v6", "2001:db8::1", 6, false, false},
		{"unparseable", "not-an-ip", 0, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Lookup(tc.ip)
			if got.Version != tc.wantVersion {
				t.Errorf("Version: got %d, want %d", got.Version, tc.wantVersion)
			}
			if got.IsPrivate != tc.wantPrivate {
				t.Errorf("IsPrivate: got %v, want %v", got.IsPrivate, tc.wantPrivate)
			}
			if got.IsLoopback != tc.wantLoop {
				t.Errorf("IsLoopback: got %v, want %v", got.IsLoopback, tc.wantLoop)
			}
		})
	}
}

func TestLookupHostPortAndBrackets(t *testing.T) {
	if got := Lookup("127.0.0.1:8080"); got.IP != "127.0.0.1" {
		t.Errorf("host:port not stripped: %q", got.IP)
	}
	if got := Lookup("[::1]"); got.IP != "::1" {
		t.Errorf("brackets not stripped: %q", got.IP)
	}
	if got := Lookup("  10.0.0.1  "); got.IP != "10.0.0.1" {
		t.Errorf("whitespace not stripped: %q", got.IP)
	}
}

func TestLookupPublicV4Geo(t *testing.T) {
	// Stable, well-known anycast — Google Public DNS. The xdb only
	// guarantees country granularity for anycast, so we just check
	// that *some* country was filled in, not the exact value.
	got := Lookup("8.8.8.8")
	if got.Country == "" {
		t.Errorf("expected non-empty Country for 8.8.8.8, got %+v", got)
	}
	if got.IsPrivate {
		t.Errorf("8.8.8.8 should not classify as private")
	}
}

func TestLookupCacheHit(t *testing.T) {
	a := Lookup("1.1.1.1")
	b := Lookup("1.1.1.1")
	if a != b {
		t.Errorf("cache returned different result on second call: %+v vs %+v", a, b)
	}
}

func TestLRUEviction(t *testing.T) {
	c := newLRU(2)
	c.put("a", Info{IP: "a"})
	c.put("b", Info{IP: "b"})
	c.put("c", Info{IP: "c"})
	if _, ok := c.get("a"); ok {
		t.Errorf("expected 'a' to be evicted")
	}
	if _, ok := c.get("b"); !ok {
		t.Errorf("expected 'b' to still be cached")
	}
	if _, ok := c.get("c"); !ok {
		t.Errorf("expected 'c' to be cached")
	}
}

func TestParseRegion(t *testing.T) {
	country, province, city, isp := parseRegion("中国|0|江苏省|苏州市|电信")
	if country != "中国" || province != "江苏省" || city != "苏州市" || isp != "电信" {
		t.Errorf("parseRegion mismatch: %q %q %q %q", country, province, city, isp)
	}
	// Zero placeholders should normalise to empty.
	country, province, city, isp = parseRegion("0|0|0|0|0")
	if country != "" || province != "" || city != "" || isp != "" {
		t.Errorf("expected all-empty for zero-only region, got %q %q %q %q", country, province, city, isp)
	}
}
