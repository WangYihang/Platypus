package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadLinuxMachineID_Fallback covers the common Linux path: if
// /etc/machine-id contains a value, readLinuxMachineID strips it clean.
func TestReadLinuxMachineID_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machine-id")
	if err := os.WriteFile(path, []byte("  abc123def\n\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readFileTrimmed(path)
	if got != "abc123def" {
		t.Fatalf("got %q; want abc123def", got)
	}
}

func TestReadLinuxMachineID_Missing(t *testing.T) {
	dir := t.TempDir()
	got := readFileTrimmed(filepath.Join(dir, "does-not-exist"))
	if got != "" {
		t.Fatalf("got %q; want empty string for missing file", got)
	}
}

// MachineID caches its result: repeated calls return the same value even
// after the underlying source would change. Matches the contract the
// handshake relies on.
func TestMachineID_IsCached(t *testing.T) {
	first := MachineID()
	second := MachineID()
	if first != second {
		t.Fatalf("MachineID() not stable: %q vs %q", first, second)
	}
}

// Lightweight smoke test: MachineID never returns a string with
// leading/trailing whitespace, regardless of OS, since downstream code
// uses it directly in DB unique keys.
func TestMachineID_NoWhitespace(t *testing.T) {
	got := MachineID()
	if got != strings.TrimSpace(got) {
		t.Fatalf("MachineID() has leading/trailing whitespace: %q", got)
	}
}
