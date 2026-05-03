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

// TestDecode_AcceptsV1Bundles: encoder is now v=2 but the decoder
// must still accept v=1 bundles produced by older servers — the
// schema bump is purely additive (BaselinePluginIDs default-nil).
func TestDecode_AcceptsV1Bundles(t *testing.T) {
	wire := "pinst_" + base64Encode(`{"v":1,"server":"h:1","pat":"plt_x.y","ca_pem":"---","project_id":"p1"}`)
	got, err := installbundle.Decode(wire)
	if err != nil {
		t.Fatalf("Decode v1: %v", err)
	}
	if got.Schema != 1 {
		t.Fatalf("Schema = %d; want 1", got.Schema)
	}
	if got.BaselinePluginIDs != nil {
		t.Fatalf("v1 BaselinePluginIDs should be nil, got %v", got.BaselinePluginIDs)
	}
	if got.Server != "h:1" || got.PAT != "plt_x.y" {
		t.Fatalf("v1 round-trip mismatch: %+v", *got)
	}
}

// TestRoundTrip_BaselinePluginIDs: schema-v2 bundles preserve the
// allowlist. The list flows from server-side install token through
// the encoded bundle into the agent's first-boot path; any drop
// here would silently re-enable the unrestricted boot mode.
func TestRoundTrip_BaselinePluginIDs(t *testing.T) {
	want := installbundle.Bundle{
		Server:            "h:1",
		PAT:               "plt_a.b",
		BaselinePluginIDs: []string{"com.platypus.sys-info", "com.platypus.sys-listdir"},
	}
	wire, err := installbundle.Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := installbundle.Decode(wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Schema != installbundle.CurrentSchema {
		t.Fatalf("Schema = %d; want %d", got.Schema, installbundle.CurrentSchema)
	}
	if len(got.BaselinePluginIDs) != 2 ||
		got.BaselinePluginIDs[0] != "com.platypus.sys-info" ||
		got.BaselinePluginIDs[1] != "com.platypus.sys-listdir" {
		t.Fatalf("BaselinePluginIDs = %v", got.BaselinePluginIDs)
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
