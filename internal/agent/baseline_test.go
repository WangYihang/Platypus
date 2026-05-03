package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestSaveLoadBaseline_RoundTrip exercises the canonical
// "operator picked some plugins on first boot, agent persists them,
// next boot reads them back" path.
func TestSaveLoadBaseline_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := []string{"com.platypus.sys-info", "com.platypus.sys-listdir"}
	if err := SaveBaseline(dir, want); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	got, err := LoadBaseline(dir)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("LoadBaseline = %v; want %v", got, want)
	}
}

// TestLoadBaseline_NotFound: missing file maps to ErrBaselineNotFound
// so callers can branch on first-boot vs returning-boot.
func TestLoadBaseline_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadBaseline(dir)
	if !errors.Is(err, ErrBaselineNotFound) {
		t.Fatalf("LoadBaseline empty dir err = %v; want ErrBaselineNotFound", err)
	}
}

// TestSaveBaseline_EmptySliceRoundTrips: operator who explicitly
// picks no plugins still gets a baseline.json on disk so subsequent
// boots don't keep re-evaluating the install bundle. The empty case
// must round-trip as a non-nil zero-length slice (vs. ErrBaselineNotFound)
// — the file's existence is the signal.
func TestSaveBaseline_EmptySliceRoundTrips(t *testing.T) {
	dir := t.TempDir()
	if err := SaveBaseline(dir, nil); err != nil {
		t.Fatalf("SaveBaseline(nil): %v", err)
	}
	got, err := LoadBaseline(dir)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if got == nil {
		t.Fatalf("LoadBaseline = nil; want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("LoadBaseline = %v; want length 0", got)
	}
}

// TestLoadBaseline_Malformed: a corrupted file surfaces as a wrapped
// JSON error, not as ErrBaselineNotFound (which would silently retry
// the install-bundle-driven path and overwrite the operator's choice).
func TestLoadBaseline_Malformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "baseline.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBaseline(dir)
	if err == nil {
		t.Fatal("LoadBaseline malformed: want error, got nil")
	}
	if errors.Is(err, ErrBaselineNotFound) {
		t.Fatalf("malformed should not equal ErrBaselineNotFound: %v", err)
	}
}
