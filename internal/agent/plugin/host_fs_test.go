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

// TestCheckFSReadPath_Glob covers the new path-glob support added in
// D3. Mixed allowlists (some literal, some glob) work together.
func TestCheckFSReadPath_Glob(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(logs, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	hit := filepath.Join(logs, "app.log")
	miss := filepath.Join(logs, "app.txt")
	deep := filepath.Join(sub, "nested.log")
	for _, f := range []string{hit, miss, deep} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name     string
		patterns []string
		path     string
		ok       bool
	}{
		{"single-star single-level hit", []string{logs + "/*.log"}, hit, true},
		{"single-star wrong ext", []string{logs + "/*.log"}, miss, false},
		{"single-star does NOT cross /", []string{logs + "/*.log"}, deep, false},
		{"double-star recurses", []string{logs + "/**/*.log"}, deep, true},
		{"double-star recurses + flat", []string{logs + "/**/*.log"}, hit, true},
		{"mixed: literal + glob, glob wins", []string{"/never/used", logs + "/*.log"}, hit, true},
		{"mixed: literal first matches", []string{logs, "/never/*"}, deep, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pctx := &pluginCtx{manifest: &Manifest{
				Capabilities: ManifestCapabilities{FSRead: &CapFSReadSpec{Paths: c.patterns}},
			}}
			_, err := pctx.checkFSReadPath(c.path)
			if c.ok {
				if err != nil {
					t.Errorf("unexpected denial: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected denial, got allow")
				}
			}
		})
	}
}

// TestCheckFSWritePath covers the write allowlist, which had no tests
// of its own until a symlink under an allowed dir was found to be a
// write portal out of it — see resolveForCreate.
func TestCheckFSWritePath(t *testing.T) {
	allowed := t.TempDir()
	other := t.TempDir()

	// Symlink under the allowed tree pointing at a directory outside
	// it. Writing *through* it must be denied at every depth.
	dirLink := filepath.Join(allowed, "leak")
	if err := os.Symlink(other, dirLink); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	pctx := &pluginCtx{manifest: &Manifest{
		Capabilities: ManifestCapabilities{FSWrite: &CapFSWriteSpec{Paths: []string{allowed}}},
	}}

	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"existing-dir", allowed, ""},
		{"new-file", filepath.Join(allowed, "new.txt"), ""},
		{"new-file-nested", filepath.Join(allowed, "a", "b", "c.txt"), ""},
		{"outside", filepath.Join(other, "x.txt"), "path_not_in_allowlist"},
		{"relative", "etc/passwd", "path_not_absolute"},

		// One missing component: the parent (the symlink itself)
		// resolved, so this was already denied.
		{"through-symlink-depth1", filepath.Join(dirLink, "a"), "path_not_in_allowlist"},
		// Two or more missing: the parent did not resolve either, and
		// the unresolved path — still prefixed with the allowed dir —
		// used to sail through the check.
		{"through-symlink-depth2", filepath.Join(dirLink, "a", "b"), "path_not_in_allowlist"},
		{"through-symlink-depth3", filepath.Join(dirLink, "a", "b", "c"), "path_not_in_allowlist"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pctx.checkFSWritePath(tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !pathHasPrefix(got, mustEval(t, allowed)) && got != mustEval(t, allowed) {
					t.Errorf("resolved %q escaped the allowlist %q", got, allowed)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %q, got allowed with resolved=%q", tc.wantErr, got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestCheckFSWritePathNoEscapeOnDisk is the end-to-end form: run the
// check, then do exactly what hostFSWriteRange does with the result,
// and confirm the bytes stayed inside the allowlist.
func TestCheckFSWritePathNoEscapeOnDisk(t *testing.T) {
	allowed := t.TempDir()
	other := t.TempDir()

	if err := os.Symlink(other, filepath.Join(allowed, "leak")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	pctx := &pluginCtx{manifest: &Manifest{
		Capabilities: ManifestCapabilities{FSWrite: &CapFSWriteSpec{Paths: []string{allowed}}},
	}}

	clean, err := pctx.checkFSWritePath(filepath.Join(allowed, "leak", "a", "b"))
	if err == nil {
		// Should not get here, but if the check ever regresses, show
		// the actual damage rather than just a mismatched error string.
		_ = os.MkdirAll(filepath.Dir(clean), 0o750)
		_ = os.WriteFile(clean, []byte("escaped"), 0o600)
		if _, err := os.Stat(filepath.Join(other, "a", "b")); err == nil {
			t.Fatalf("write escaped the allowlist: landed in %s", other)
		}
		t.Fatalf("check allowed a write through a symlink out of %s", allowed)
	}
	if !strings.Contains(err.Error(), "path_not_in_allowlist") {
		t.Errorf("got %v, want path_not_in_allowlist", err)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", p, err)
	}
	return got
}
