package keychain

import (
	"errors"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

func init() {
	// Ensure tests never touch the real OS keychain.
	keyring.MockInit()
}

func TestStore_SaveLoadRoundTrip(t *testing.T) {
	keyring.MockInit()
	s := New("platypus-desktop-test")

	if err := s.Save("local", "the-secret"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load("local")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != "the-secret" {
		t.Errorf("Load = %q, want %q", got, "the-secret")
	}
}

func TestStore_SaveOverwrites(t *testing.T) {
	keyring.MockInit()
	s := New("platypus-desktop-test")

	if err := s.Save("p", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("p", "v2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("p")
	if err != nil {
		t.Fatal(err)
	}
	if got != "v2" {
		t.Errorf("Load = %q, want v2", got)
	}
}

func TestStore_LoadMissing(t *testing.T) {
	keyring.MockInit()
	s := New("platypus-desktop-test")

	_, err := s.Load("does-not-exist")
	if err == nil {
		t.Fatal("expected error on missing key")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestStore_Delete(t *testing.T) {
	keyring.MockInit()
	s := New("platypus-desktop-test")

	if err := s.Save("p", "v"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("p"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Load("p")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete, Load err = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteMissing(t *testing.T) {
	keyring.MockInit()
	s := New("platypus-desktop-test")

	// Idempotent: deleting a missing key should not error.
	if err := s.Delete("missing"); err != nil {
		t.Errorf("Delete on missing key returned %v, want nil", err)
	}
}

func TestStore_ServiceIsolation(t *testing.T) {
	keyring.MockInit()
	a := New("svc-a")
	b := New("svc-b")

	if err := a.Save("k", "value-from-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Load("k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("svc-b should not see svc-a's keys; err = %v", err)
	}
}
