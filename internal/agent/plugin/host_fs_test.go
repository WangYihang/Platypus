package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathHasPrefix(t *testing.T) {
	cases := []struct {
		p, prefix string
		want      bool
	}{
		{"/etc/nginx", "/etc/nginx", true},
		{"/etc/nginx/foo", "/etc/nginx", true},
		{"/etc/nginx/foo/bar", "/etc/nginx", true},
		{"/etc/nginx2", "/etc/nginx", false}, // sibling, not descendant
		{"/etc", "/etc/nginx", false},
		{"/var/log", "/etc/nginx", false},
		// Root prefix is everyday "give the plugin full read access" —
		// this matches the system ListDir / Stat allowlist; without
		// the special case in pathHasPrefix it would deny every
		// non-root absolute path.
		{"/tmp/foo", "/", true},
		{"/etc/nginx/nginx.conf", "/", true},
		{"/", "/", true},
	}
	for _, tc := range cases {
		t.Run(tc.p+"|"+tc.prefix, func(t *testing.T) {
			if got := pathHasPrefix(tc.p, tc.prefix); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCheckFSReadPath(t *testing.T) {
	allowed := t.TempDir()
	other := t.TempDir()

	// File inside the allowed tree.
	fInside := filepath.Join(allowed, "ok.txt")
	if err := os.WriteFile(fInside, []byte("hi"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// File outside.
	fOutside := filepath.Join(other, "secret.txt")
	if err := os.WriteFile(fOutside, []byte("nope"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Symlink under allowed pointing outside — must be rejected.
	linkPath := filepath.Join(allowed, "leak")
	if err := os.Symlink(fOutside, linkPath); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	pctx := &pluginCtx{manifest: &Manifest{
		Capabilities: ManifestCapabilities{FSRead: &CapFSReadSpec{Paths: []string{allowed}}},
	}}

	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"inside", fInside, ""},
		{"outside", fOutside, "path_not_in_allowlist"},
		{"symlink-out", linkPath, "path_not_in_allowlist"},
		{"relative", "etc/nginx", "path_not_absolute"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pctx.checkFSReadPath(tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}

	// Capability completely missing → denied even for in-tree paths.
	pctx2 := &pluginCtx{manifest: &Manifest{}}
	if _, err := pctx2.checkFSReadPath(fInside); err == nil ||
		!strings.Contains(err.Error(), "capability_denied") {
		t.Errorf("expected capability_denied, got %v", err)
	}
}
