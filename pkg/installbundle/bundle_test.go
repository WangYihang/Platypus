package installbundle_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/pkg/installbundle"
)

func TestRoundTrip(t *testing.T) {
	want := installbundle.Bundle{
		Server:    "platypus.corp:9443",
		PAT:       "plt_abc.def",
		CACertPEM: "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n",
		ProjectID: "default",
	}
	wire, err := installbundle.Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(wire, installbundle.Prefix) {
		t.Fatalf("missing prefix: %q", wire)
	}
	got, err := installbundle.Decode(wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Server != want.Server || got.PAT != want.PAT ||
		got.CACertPEM != want.CACertPEM || got.ProjectID != want.ProjectID {
		t.Fatalf("round-trip mismatch:\n got=%+v\nwant=%+v", *got, want)
	}
	if got.Schema != installbundle.CurrentSchema {
		t.Fatalf("Schema = %d, want %d", got.Schema, installbundle.CurrentSchema)
	}
}

func TestEncode_RequiresServerAndPAT(t *testing.T) {
	_, err := installbundle.Encode(installbundle.Bundle{PAT: "plt_x.y"})
	if err == nil {
		t.Fatal("Encode without Server should error")
	}
	_, err = installbundle.Encode(installbundle.Bundle{Server: "h:1"})
	if err == nil {
		t.Fatal("Encode without PAT should error")
	}
}

// TestDecode_ErrorPaths exercises the parse failure modes the agent
// CLI maps to a friendly "doesn't look like an install bundle" error.
func TestDecode_ErrorPaths(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"no prefix", "plt_abc.def"},
		{"bad base64", "pinst_!!!"},
		{"bad json", "pinst_" + base64Encode(`not json`)},
		{"missing fields", "pinst_" + base64Encode(`{"v":1}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := installbundle.Decode(tc.in)
			if err == nil {
				t.Fatalf("want error, got nil")
			}
			if !errors.Is(err, installbundle.ErrMalformed) {
				t.Fatalf("want ErrMalformed, got %v", err)
			}
		})
	}
}

// TestDecode_ForwardCompatGuard: a v=99 bundle from a future server
// must not be silently parsed as a known schema.
func TestDecode_ForwardCompatGuard(t *testing.T) {
	wire := "pinst_" + base64Encode(`{"v":99,"server":"h:1","pat":"plt_x.y"}`)
	if _, err := installbundle.Decode(wire); err == nil {
		t.Fatal("future schema should error")
	}
}

// TestLooks: prefix-only check.
func TestLooks(t *testing.T) {
	if !installbundle.Looks("pinst_abc") {
		t.Fatal("pinst_abc should Looks==true")
	}
	if installbundle.Looks("plt_abc.def") {
		t.Fatal("plt_ should not Looks like a bundle")
	}
	if installbundle.Looks("") {
		t.Fatal("empty should not Looks")
	}
}

// TestWireURL Safety: the encoded body uses RawURLEncoding so the
// token can ride in a URL path / query without further escaping.
// Spot-check by encoding a worst-case payload (lots of '/' '+' '=' in
// the underlying JSON) and verifying the wire form contains none of
// the URL-unsafe alphabet.
func TestWireURLSafety(t *testing.T) {
	wire, err := installbundle.Encode(installbundle.Bundle{
		Server: "h:1", PAT: "plt_a.b",
		CACertPEM: strings.Repeat("//==++", 32),
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	body := strings.TrimPrefix(wire, installbundle.Prefix)
	for _, ch := range body {
		switch ch {
		case '/', '+', '=':
			t.Fatalf("wire contains url-unsafe %q: %q", ch, wire)
		}
	}
}

// base64Encode is a tiny helper that avoids pulling encoding/base64
// into every individual test case.
func base64Encode(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
