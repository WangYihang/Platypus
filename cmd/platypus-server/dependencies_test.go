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

// minDesktopToolchainMinor / minDesktopToolchainPatch are the floor
// the desktop module's build must meet, as a (minor, patch) pair
// compared lexicographically. Both halves matter: a bare patch floor
// would reject a perfectly good go1.27.0 for having patch 0, and a
// bare minor floor would let the module drift back onto an unpatched
// .0 of the minor it is already on.
//
// Update these constants and the version lines in go.mod and
// desktop/go.mod together when you intentionally raise the floor —
// tying the assertion to a single source of truth keeps drift loud.
const (
	minDesktopToolchainMinor = 27
	minDesktopToolchainPatch = 0
)

// TestDesktopGoModEffectiveToolchainFloor guards the stdlib the desktop
// submodule actually builds against.
//
// The concrete regression: the module once declared `go 1.25.0` while
// the patch-level stdlib fixes it needed had landed in 1.25.9, so a
// contributor on a stock 1.25.0 toolchain inherited all 16 vulns
// govulncheck reports (GO-2025-4007 through GO-2026-4947 across
// crypto/x509, crypto/tls, net/url, net/http, encoding/asn1,
// encoding/pem, os). A `toolchain go1.25.9` line closed that gap.
//
// What matters is therefore the *effective* floor — max(go directive,
// toolchain pin) — not the presence of any particular line. An earlier
// version of this test demanded a literal `toolchain` line, which is
// wrong in both directions: `go mod tidy` deletes that line as
// redundant whenever it equals the `go` directive, and a `toolchain`
// line alone says nothing if the `go` directive is what's too low.
func TestDesktopGoModEffectiveToolchainFloor(t *testing.T) {
	repoRoot := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(repoRoot, "desktop", "go.mod"))
	if err != nil {
		t.Fatalf("read desktop/go.mod: %v", err)
	}
	text := string(body)

	goPat := regexp.MustCompile(`(?m)^go\s+1\.(\d+)(?:\.(\d+))?\b`)
	m := goPat.FindStringSubmatch(text)
	if m == nil {
		t.Fatal("desktop/go.mod has no `go 1.X[.Y]` directive")
	}
	minor, patch := atoi(t, m[1]), 0
	if m[2] != "" {
		patch = atoi(t, m[2])
	}

	// A toolchain pin only ever raises the effective floor; Go ignores
	// one that is lower than the `go` directive.
	tcPat := regexp.MustCompile(`(?m)^toolchain\s+go1\.(\d+)\.(\d+)\b`)
	if tc := tcPat.FindStringSubmatch(text); tc != nil {
		tcMinor, tcPatch := atoi(t, tc[1]), atoi(t, tc[2])
		if tcMinor > minor || (tcMinor == minor && tcPatch > patch) {
			minor, patch = tcMinor, tcPatch
		}
	}

	if minor < minDesktopToolchainMinor ||
		(minor == minDesktopToolchainMinor && patch < minDesktopToolchainPatch) {
		t.Fatalf(
			"desktop/go.mod builds against go1.%d.%d at the earliest, but the "+
				"minimum is go1.%d.%d — raise the `go` directive, or add a "+
				"`toolchain go1.%d.%d` line under it when the language version "+
				"needs to stay where it is. Below the floor the build inherits "+
				"the patch-level stdlib vulnerabilities govulncheck reports. "+
				"(If this bump is intentional, raise the minDesktopToolchain* "+
				"constants in this test too.)",
			minor, patch,
			minDesktopToolchainMinor, minDesktopToolchainPatch,
			minDesktopToolchainMinor, minDesktopToolchainPatch,
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
			`desktop/frontend/package.json does mention postcss but the version `+
				`range doesn't pin >= 8.5.10. The override must force at least `+
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
