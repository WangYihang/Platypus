package system

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveSource_FallsBackToEmbedWhenDataDirEmpty: caller hasn't
// resolved a data dir yet (early-boot before bootstrap). Resolver
// must pick the embedded tree without panicking on the empty path.
func TestResolveSource_FallsBackToEmbedWhenDataDirEmpty(t *testing.T) {
	got, err := ResolveSource("")
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	if got.Origin != "embedded" {
		t.Fatalf("Origin = %q; want embedded", got.Origin)
	}
	if got.FS == nil {
		t.Fatal("FS = nil")
	}
}

// TestResolveSource_NoOverridePresent: data dir is real but doesn't
// contain a system-plugins/ tree. Resolver must NOT touch the
// non-existent override and just pick the embedded tree.
func TestResolveSource_NoOverridePresent(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveSource(dir)
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	if got.Origin != "embedded" {
		t.Fatalf("Origin = %q; want embedded", got.Origin)
	}
}

// TestResolveSource_OverrideMissingPublisher: the override dir exists
// (something staged a tree there) but no publisher.pub at the root —
// EnsureInstalled would reject every bundle anyway, so the resolver
// short-circuits to embedded so the boot logs are honest about
// which key set the agent is verifying against.
func TestResolveSource_OverrideMissingPublisher(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, OverrideSubdir, "com.example.x", "1.0.0"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveSource(dir)
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	if got.Origin != "embedded" {
		t.Fatalf("override-without-publisher should fall back; Origin = %q", got.Origin)
	}
}

// TestResolveSource_OverrideHonoured: data dir contains a
// well-formed override (publisher.pub at the root). Resolver picks
// it and exposes the override's FS, NOT the embedded tree.
//
// We verify the FS by checking that publisher.pub reads back the
// bytes we wrote — the embedded tree would have its own (different)
// publisher.pub at that path.
func TestResolveSource_OverrideHonoured(t *testing.T) {
	dir := t.TempDir()
	overrideRoot := filepath.Join(dir, OverrideSubdir)
	if err := os.MkdirAll(overrideRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	pubBytes := []byte("untrusted comment: dev override\nMARKER")
	if err := os.WriteFile(filepath.Join(overrideRoot, "publisher.pub"), pubBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSource(dir)
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	if got.Origin != "data-dir" {
		t.Fatalf("Origin = %q; want data-dir", got.Origin)
	}
	read, err := fs.ReadFile(got.FS, "publisher.pub")
	if err != nil {
		t.Fatalf("read publisher.pub from override: %v", err)
	}
	if string(read) != string(pubBytes) {
		t.Fatalf("override publisher.pub mismatch:\n got=%q\nwant=%q", read, pubBytes)
	}
}
