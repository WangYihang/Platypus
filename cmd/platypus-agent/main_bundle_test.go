package main

import (
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/pkg/installbundle"
	"github.com/WangYihang/Platypus/pkg/options"
)

// TestExpandInstallBundle_HappyPath verifies the bundle expander
// rewrites Token / RemoteHost / RemotePort and surfaces the CA bytes
// to the caller.
func TestExpandInstallBundle_HappyPath(t *testing.T) {
	wire, err := installbundle.Encode(installbundle.Bundle{
		Server:    "agent.corp:9443",
		PAT:       "plt_abc.def",
		CACertPEM: "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n",
		ProjectID: "demo",
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	opts := &options.Options{Token: wire}

	caPEM, err := expandInstallBundle(opts)
	if err != nil {
		t.Fatalf("expandInstallBundle: %v", err)
	}
	if opts.Token != "plt_abc.def" {
		t.Errorf("Token = %q, want plt_abc.def", opts.Token)
	}
	if opts.RemoteHost != "agent.corp" || opts.RemotePort != 9443 {
		t.Errorf("server = %s:%d, want agent.corp:9443", opts.RemoteHost, opts.RemotePort)
	}
	if !strings.HasPrefix(string(caPEM), "-----BEGIN CERTIFICATE-----") {
		t.Errorf("CA bytes not surfaced: %q", string(caPEM))
	}
}

// TestExpandInstallBundle_ExplicitFlagsWin: a --host / --port pair on
// the CLI overrides the bundle's server endpoint. Use case: admin
// debugging a bundle generated against a different staging server.
func TestExpandInstallBundle_ExplicitFlagsWin(t *testing.T) {
	wire, _ := installbundle.Encode(installbundle.Bundle{
		Server: "bundle.corp:1111",
		PAT:    "plt_x.y",
	})
	opts := &options.Options{
		Token:      wire,
		RemoteHost: "override.corp",
		RemotePort: 2222,
	}
	if _, err := expandInstallBundle(opts); err != nil {
		t.Fatalf("expandInstallBundle: %v", err)
	}
	if opts.RemoteHost != "override.corp" || opts.RemotePort != 2222 {
		t.Fatalf("flag override lost: server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
}

// TestExpandInstallBundle_NonBundleNoOp: a plain `plt_` token (legacy
// flow) leaves opts untouched and returns no CA override.
func TestExpandInstallBundle_NonBundleNoOp(t *testing.T) {
	opts := &options.Options{
		Token:      "plt_x.y",
		RemoteHost: "h",
		RemotePort: 1,
	}
	caPEM, err := expandInstallBundle(opts)
	if err != nil {
		t.Fatalf("expandInstallBundle: %v", err)
	}
	if caPEM != nil {
		t.Errorf("non-bundle token should not produce CA override: %q", string(caPEM))
	}
	if opts.Token != "plt_x.y" || opts.RemoteHost != "h" || opts.RemotePort != 1 {
		t.Errorf("opts mutated: %+v", opts)
	}
}

// TestExpandInstallBundle_Malformed: a `pinst_` prefix that doesn't
// decode is a hard parse error so the user gets a contextual usage
// hint instead of a silent fallback to "no token".
func TestExpandInstallBundle_Malformed(t *testing.T) {
	opts := &options.Options{Token: "pinst_!!!notbase64"}
	if _, err := expandInstallBundle(opts); err == nil {
		t.Fatal("malformed bundle should error")
	}
}

