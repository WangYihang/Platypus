package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolvePSKFile_Priority pins the documented resolution order:
// CLI flag > env file > env inline > data-dir default > /etc fallback.
func TestResolvePSKFile_Priority(t *testing.T) {
	t.Run("cli wins", func(t *testing.T) {
		td := t.TempDir()
		got, err := ResolvePSKFile(PSKResolveOptions{
			CLIPath:   "/explicit.psk",
			EnvFile:   "/from-env.psk",
			EnvInline: "should-not-be-used",
			DataDir:   td,
		})
		if err != nil || got != "/explicit.psk" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})
	t.Run("env file wins over inline + data-dir", func(t *testing.T) {
		td := t.TempDir()
		// Place a file at the data-dir default so we know env-file
		// is winning, not data-dir.
		_ = os.WriteFile(filepath.Join(td, PSKFileName), []byte("X\n"), 0o600)
		got, err := ResolvePSKFile(PSKResolveOptions{
			EnvFile: "/from-env.psk",
			DataDir: td,
		})
		if err != nil || got != "/from-env.psk" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})
	t.Run("env inline materialises and wins over data-dir", func(t *testing.T) {
		td := t.TempDir()
		_ = os.WriteFile(filepath.Join(td, PSKFileName), []byte("DATA-DIR\n"), 0o600)

		ephemeral := t.TempDir()
		got, err := ResolvePSKFile(PSKResolveOptions{
			EnvInline:    "AAAABBBBCCCC",
			DataDir:      td,
			EphemeralDir: ephemeral,
		})
		if err != nil {
			t.Fatalf("ResolvePSKFile: %v", err)
		}
		if !strings.HasPrefix(got, ephemeral) {
			t.Fatalf("inline path should land under ephemeral dir: got=%q ephemeral=%q", got, ephemeral)
		}
		body, _ := os.ReadFile(got)
		if strings.TrimSpace(string(body)) != "AAAABBBBCCCC" {
			t.Fatalf("inline contents not written: %q", body)
		}
		// 0600 permission check.
		fi, _ := os.Stat(got)
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("perm = %o, want 0o600", fi.Mode().Perm())
		}
	})
	t.Run("data-dir wins when nothing higher set", func(t *testing.T) {
		td := t.TempDir()
		target := filepath.Join(td, PSKFileName)
		_ = os.WriteFile(target, []byte("X\n"), 0o600)
		got, err := ResolvePSKFile(PSKResolveOptions{DataDir: td})
		if err != nil || got != target {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})
	t.Run("nothing configured returns empty", func(t *testing.T) {
		td := t.TempDir() // empty
		got, err := ResolvePSKFile(PSKResolveOptions{DataDir: td})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		// Could legitimately match systemPSKPath if the test host
		// happens to have one. Tolerate that — assert only that we
		// didn't fabricate a path under the empty data-dir.
		if got != "" && got != systemPSKPath {
			t.Fatalf("got=%q, want empty or %s", got, systemPSKPath)
		}
	})
}

// TestInstallPSK_HappyPath writes a PSK and verifies normalisation
// (whitespace collapsed) and 0600 permission.
func TestInstallPSK_HappyPath(t *testing.T) {
	td := t.TempDir()
	target := filepath.Join(td, "mesh.psk")
	if err := InstallPSK(target, "  AAAA  BBBB \n CCCC\n"); err != nil {
		t.Fatalf("InstallPSK: %v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.TrimSpace(string(body)) != "AAAABBBBCCCC" {
		t.Fatalf("contents = %q (want whitespace-collapsed)", body)
	}
	fi, _ := os.Stat(target)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0o600", fi.Mode().Perm())
	}
}

// TestInstallPSK_EmptyRefused: an empty input is a parse error so
// `psk install ""` doesn't silently clear the existing PSK.
func TestInstallPSK_EmptyRefused(t *testing.T) {
	td := t.TempDir()
	if err := InstallPSK(filepath.Join(td, "mesh.psk"), "   \n   "); err == nil {
		t.Fatal("InstallPSK should refuse empty input")
	}
}

// TestInstallPSK_OverwriteIsAtomic: re-installing a different PSK
// over an existing file replaces the contents without ever leaving
// a half-written file behind. We check by writing a known marker
// then overwriting and reading back.
func TestInstallPSK_OverwriteIsAtomic(t *testing.T) {
	td := t.TempDir()
	target := filepath.Join(td, "mesh.psk")
	if err := InstallPSK(target, "OLD"); err != nil {
		t.Fatalf("install old: %v", err)
	}
	if err := InstallPSK(target, "NEW"); err != nil {
		t.Fatalf("install new: %v", err)
	}
	body, _ := os.ReadFile(target)
	if strings.TrimSpace(string(body)) != "NEW" {
		t.Fatalf("contents = %q", body)
	}
	// No leftover .tmp.
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("leftover .tmp file: %v", err)
	}
}
