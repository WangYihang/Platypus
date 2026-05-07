package storage_test

import (
	"context"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/cryptobox"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// setKEKForTest installs a deterministic 32-byte KEK for the duration
// of one test. Real production paths read PLATYPUS_CA_KEK; tests use
// t.Setenv so the tear-down is automatic. Without this any Seal call
// would return ErrKEKMissing.
func setKEKForTest(t *testing.T) {
	t.Helper()
	// 64 hex chars (32 bytes). Value is irrelevant — tests only need
	// determinism within one process; sharing a constant across
	// tests is fine because the env is reset between cases.
	t.Setenv(cryptobox.EnvVar,
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
}

func TestProjectSecrets_CreateAndReveal(t *testing.T) {
	setKEKForTest(t)
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	plain := []byte("super-secret-value")
	s, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "datadog_api_key", "DD intake", admin.ID, plain,
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.SecretID == "" || !strings.HasPrefix(s.SecretID, "sec_") {
		t.Fatalf("SecretID = %q", s.SecretID)
	}
	// Stored row carries ciphertext + nonce, not plaintext.
	if string(s.Ciphertext) == string(plain) {
		t.Fatalf("ciphertext equals plaintext — encryption did not run")
	}
	if len(s.Nonce) != cryptobox.NonceLen {
		t.Fatalf("nonce length = %d, want %d", len(s.Nonce), cryptobox.NonceLen)
	}

	// Reveal round-trips back to the original plaintext.
	got, err := db.ProjectSecrets().Reveal(context.Background(), s.SecretID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("Reveal returned %q, want %q", got, plain)
	}
}

func TestProjectSecrets_Reveal_Missing(t *testing.T) {
	setKEKForTest(t)
	db := newTestDB(t)
	if _, err := db.ProjectSecrets().Reveal(context.Background(), "sec_nope"); err != storage.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestProjectSecrets_Reveal_RefusesRevoked(t *testing.T) {
	setKEKForTest(t)
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	s, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "to-revoke", "", admin.ID, []byte("v"),
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := db.ProjectSecrets().Revoke(
		context.Background(), s.SecretID, admin.ID, "rotated",
	); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	// Revealing a revoked secret refuses — callers should surface
	// "rotate the plugin config to use the new id" rather than
	// silently using a stale value.
	if _, err := db.ProjectSecrets().Reveal(context.Background(), s.SecretID); err == nil {
		t.Fatalf("Reveal on revoked secret should fail")
	}
	// Revoking again is idempotent.
	if err := db.ProjectSecrets().Revoke(
		context.Background(), s.SecretID, admin.ID, "again",
	); err != nil {
		t.Fatalf("idempotent Revoke: %v", err)
	}
}

func TestProjectSecrets_ListRedacts(t *testing.T) {
	setKEKForTest(t)
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	if _, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "a", "", admin.ID, []byte("av"),
	); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	if _, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "b", "", admin.ID, []byte("bv"),
	); err != nil {
		t.Fatalf("Create b: %v", err)
	}

	rows, err := db.ProjectSecrets().ListByProject(context.Background(), proj.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	// ProjectSecretRedacted has no fields exposing nonce or
	// ciphertext — the type system enforces redaction. The presence
	// of identifying metadata is enough.
	for _, r := range rows {
		if r.Name == "" || r.SecretID == "" {
			t.Fatalf("row missing identity: %+v", r)
		}
	}
}

func TestProjectSecrets_DuplicateNameWhileActive(t *testing.T) {
	setKEKForTest(t)
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	if _, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "dup", "", admin.ID, []byte("v"),
	); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	// The partial UNIQUE index rejects a second active row with the
	// same (project_id, name) — operators see the conflict and
	// either pick a different name or revoke first.
	if _, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "dup", "", admin.ID, []byte("v2"),
	); err == nil {
		t.Fatalf("expected UNIQUE conflict on duplicate active name")
	}
}

func TestProjectSecrets_ReuseNameAfterRevoke(t *testing.T) {
	setKEKForTest(t)
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	first, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "rotating", "", admin.ID, []byte("v1"),
	)
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if err := db.ProjectSecrets().Revoke(
		context.Background(), first.SecretID, admin.ID, "rotation",
	); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	// Once the old row is revoked, its (project_id, name) is no
	// longer indexed by the partial UNIQUE — a fresh row with the
	// same name succeeds.
	second, err := db.ProjectSecrets().Create(
		context.Background(), proj.ID, "rotating", "", admin.ID, []byte("v2"),
	)
	if err != nil {
		t.Fatalf("Create 2 (post-revoke): %v", err)
	}
	if second.SecretID == first.SecretID {
		t.Fatalf("rotation produced same secret_id")
	}
	got, err := db.ProjectSecrets().Reveal(context.Background(), second.SecretID)
	if err != nil {
		t.Fatalf("Reveal new: %v", err)
	}
	if string(got) != "v2" {
		t.Fatalf("Reveal new = %q, want %q", got, "v2")
	}
}
