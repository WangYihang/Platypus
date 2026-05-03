package plugin

import (
	"crypto/sha256"
	"strings"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestValidateInstallRequest(t *testing.T) {
	good := func() *v2pb.PluginInstallRequest {
		return &v2pb.PluginInstallRequest{
			PluginId:        "com.example.foo",
			Version:         "1.0.0",
			PublisherPubkey: []byte("dummy"),
			Source:          &v2pb.PluginInstallRequest_Inline{Inline: &v2pb.PluginInlineSource{}},
		}
	}
	cases := []struct {
		name string
		mut  func(*v2pb.PluginInstallRequest)
		err  string
	}{
		{"happy", func(*v2pb.PluginInstallRequest) {}, ""},
		{"bad id", func(r *v2pb.PluginInstallRequest) { r.PluginId = "Bad ID" }, "not a valid reverse-DNS"},
		{"bad version", func(r *v2pb.PluginInstallRequest) { r.Version = "1.0" }, "not strict semver"},
		{"missing pubkey", func(r *v2pb.PluginInstallRequest) { r.PublisherPubkey = nil }, "publisher_pubkey is required"},
		{"missing source", func(r *v2pb.PluginInstallRequest) { r.Source = nil }, "source is required"},
		{"url missing fields", func(r *v2pb.PluginInstallRequest) {
			r.Source = &v2pb.PluginInstallRequest_Url{Url: &v2pb.PluginURLSource{WasmUrl: "https://example/x"}}
		}, "url source requires"},
		{"url bad sha", func(r *v2pb.PluginInstallRequest) {
			r.Source = &v2pb.PluginInstallRequest_Url{Url: &v2pb.PluginURLSource{
				WasmUrl: "u", SignatureUrl: "s", ManifestUrl: "m", WasmSha256: []byte("short"),
			}}
		}, "must be 32 bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := good()
			tc.mut(r)
			err := validateInstallRequest(r)
			if tc.err == "" {
				if err != nil {
					t.Errorf("unexpected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("err = %v, want substring %q", err, tc.err)
			}
		})
	}
}

func TestVerifySha256(t *testing.T) {
	data := []byte("hello world")
	got := sha256.Sum256(data)
	if err := verifySha256(data, got[:]); err != nil {
		t.Errorf("happy: %v", err)
	}
	bad := append([]byte{}, got[:]...)
	bad[0] ^= 1
	if err := verifySha256(data, bad); err == nil {
		t.Errorf("expected mismatch error")
	}
	if err := verifySha256(data, []byte("short")); err == nil {
		t.Errorf("expected length error")
	}
}
