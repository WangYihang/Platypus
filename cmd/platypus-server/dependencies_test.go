package main_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Regression tests for the dependency-hygiene state of the repo.
//
// Both modules in this repo (root + desktop/) and the SPA's
// package.json have been historically affected by transitive vulns
// that landed via toolchain / package-manager defaults rather than
// any direct require. This file pins the floors that matter so a
// future "I'll just bump go.mod by hand" or "let me remove that
// pnpm override, who needs it" regression fails the test rather
// than reaching production.
//
// The tests are deliberately stringy (file-content assertions) so
// they can run without network access — they don't actually re-run
// govulncheck or pnpm audit, they just lock in the rule those
// tools would have caught.

// minToolchainPatch is the floor the desktop module must declare.
// Update both this constant and the toolchain line in
// desktop/go.mod when you intentionally raise the floor — tying
// the assertion to a single source of truth keeps drift loud.
const minDesktopToolchainPatch = 9

// TestDesktopGoModHasToolchainPin guards against the desktop
// submodule reverting to a bare `go 1.25.x` directive without a
// `toolchain go1.25.<patch>` pin. Without the pin the build picks
// the language-version stdlib (1.25.0) and inherits the 16
// patch-level stdlib vulnerabilities that govulncheck reports
// (GO-2025-4007 through GO-2026-4947 across crypto/x509, crypto/tls,
// net/url, net/http, encoding/asn1, encoding/pem, os).
func TestDesktopGoModHasToolchainPin(t *testing.T) {
	repoRoot := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(repoRoot, "desktop", "go.mod"))
	if err != nil {
		t.Fatalf("read desktop/go.mod: %v", err)
	}

	// Look for `toolchain go1.<minor>.<patch>` on its own line.
	// We don't pin a specific minor (1.25 today, possibly 1.26
	// later), only the floor on the patch within whatever minor.
	tcPat := regexp.MustCompile(`(?m)^toolchain\s+go1\.\d+\.(\d+)\b`)
	m := tcPat.FindStringSubmatch(string(body))
	if m == nil {
		t.Fatalf(
			"desktop/go.mod has no `toolchain go1.X.Y` line; without it " +
				"the build pins to the language-version stdlib and inherits " +
				"the patch-level stdlib vulnerabilities govulncheck reports. " +
				"Add `toolchain go1.25.%d` (or newer) immediately under the `go` line.",
			minDesktopToolchainPatch,
		)
	}
	patch := atoi(t, m[1])
	if patch < minDesktopToolchainPatch {
		t.Fatalf(
			"desktop/go.mod toolchain patch is go1.X.%d but minimum is "+
				"go1.X.%d (raise both the file and minDesktopToolchainPatch in "+
				"this test if you intentionally floor higher)",
			patch, minDesktopToolchainPatch,
		)
	}
}

// TestFrontendPostcssOverride locks the pnpm.overrides entry that
// forces postcss >= 8.5.10 transitively, regardless of what
// `geist > next > postcss` happens to resolve to. Without the
// override pnpm picks the satisfiable-but-vulnerable 8.4.31 and
// triggers GHSA-qx2v-qp2m-jg93 (XSS via unescaped </style>).
func TestFrontendPostcssOverride(t *testing.T) {
	repoRoot := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(repoRoot, "desktop", "frontend", "package.json"))
	if err != nil {
		t.Fatalf("read desktop/frontend/package.json: %v", err)
	}
	text := string(body)

	if !strings.Contains(text, `"postcss"`) {
		t.Fatal(
			`desktop/frontend/package.json does not mention "postcss" anywhere; ` +
				`a pnpm.overrides entry like "postcss": ">=8.5.10" is required to ` +
				`pin the transitive dep against GHSA-qx2v-qp2m-jg93.`,
		)
	}
	// Loose match: the override block must declare a postcss range
	// of >=8.5.10 (or newer). We accept either ">=8.5.10" or a
	// single-version pin that's already past the fix line.
	rangeRe := regexp.MustCompile(`"postcss"\s*:\s*"[^"]*8\.5\.(1\d|[2-9]\d?)\d*"|"postcss"\s*:\s*">=\s*8\.5\.(1\d|[2-9])"`)
	if !rangeRe.MatchString(text) {
		t.Fatalf(
			`desktop/frontend/package.json does mention postcss but the version ` +
				`range doesn't pin >= 8.5.10. The override must force at least ` +
				`8.5.10 to dodge GHSA-qx2v-qp2m-jg93. Current package.json text: %s`,
			snippetAround(text, `"postcss"`),
		)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// `go.mod` lives at the repo root and at desktop/. The
			// root one is the larger of the two (more lines), but
			// we only need to find the *containing* root, which is
			// the topmost go.mod walking up.
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod walking up from %s", dir)
		}
		dir = parent
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("non-digit in toolchain patch: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// snippetAround returns ~80 chars surrounding the first occurrence
// of needle so the test failure message points the operator at the
// offending line without dumping the whole file.
func snippetAround(text, needle string) string {
	i := strings.Index(text, needle)
	if i < 0 {
		return "<not found>"
	}
	start := i - 40
	if start < 0 {
		start = 0
	}
	end := i + 80
	if end > len(text) {
		end = len(text)
	}
	return text[start:end]
}
