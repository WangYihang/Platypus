package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
)

// verifierTestSetup creates an in-memory db, seeds a user, creates a
// user-session token, and returns the verifier together with the
// session's plaintext. UserSession is the only opaque-token kind in
// auth_tokens today (post-AAT removal); it exercises every dispatch /
// cache / DB-reason path on the verifier.
func verifierTestSetup(t *testing.T) (*api.TokenVerifier, *storage.DB, string, *storage.UserSession) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES ('u1','u1','x','admin',?)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	id, _, hash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatalf("optoken.Generate: %v", err)
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID:       id,
		SecretHash:    hash,
		UserID:        "u1",
		CreatedAt:     now,
		ExpiresAt:     now.Add(30 * 24 * time.Hour),
		IdleExpiresAt: now.Add(time.Hour),
		UserAgent:     "verifier-test",
	}
	if err := db.AuthTokens().CreateSession(context.Background(), s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	cache := optoken.NewCache(64, 30*time.Second)
	v := api.NewTokenVerifier(db, cache)
	return v, db, plaintext, s
}

func TestTokenVerifier_Success(t *testing.T) {
	t.Parallel()
	v, _, plaintext, s := verifierTestSetup(t)

	p, reason, err := v.Verify(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("Verify err: %v", err)
	}
	if reason != "success" {
		t.Errorf("reason = %q, want success", reason)
	}
	if p == nil || p.Kind != api.PrincipalUser {
		t.Fatalf("principal = %+v, want PrincipalUser", p)
	}
	if p.UserID != "u1" {
		t.Errorf("identity mismatch: %+v", p)
	}
	_ = s
}

func TestTokenVerifier_UnrecognizedPrefix(t *testing.T) {
	t.Parallel()
	v, _, _, _ := verifierTestSetup(t)
	p, reason, err := v.Verify(context.Background(), "eyJhbGc.foo.bar") // looks like JWT
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if p != nil {
		t.Errorf("p = %+v, want nil", p)
	}
	if reason != "unrecognized" {
		t.Errorf("reason = %q, want unrecognized", reason)
	}
}

func TestTokenVerifier_Malformed(t *testing.T) {
	t.Parallel()
	v, _, _, _ := verifierTestSetup(t)
	cases := []string{
		"pst_",
		"pst_only-id-no-dot",
		"pst_id.!!notbase32!!",
	}
	for _, raw := range cases {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			p, reason, err := v.Verify(context.Background(), raw)
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if p != nil {
				t.Errorf("p = %+v, want nil", p)
			}
			if reason != "malformed" {
				t.Errorf("reason = %q, want malformed", reason)
			}
		})
	}
}

func TestTokenVerifier_DBReasons(t *testing.T) {
	t.Parallel()

	t.Run("revoked", func(t *testing.T) {
		t.Parallel()
		v, db, plaintext, s := verifierTestSetup(t)
		if err := db.AuthTokens().Revoke(context.Background(), s.TokenID, "u1", "test", time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		_, reason, _ := v.Verify(context.Background(), plaintext)
		if reason != "revoked" {
			t.Errorf("reason = %q, want revoked", reason)
		}
	})

	// Expired: set ExpiresAt in the past via direct UPDATE so the
	// CHECK constraint stays satisfied.
	t.Run("expired", func(t *testing.T) {
		t.Parallel()
		v, db, plaintext, s := verifierTestSetup(t)
		if _, err := db.Exec(
			`UPDATE auth_tokens SET expires_at = ? WHERE token_id = ?`,
			time.Now().Add(-time.Minute).UTC(), s.TokenID,
		); err != nil {
			t.Fatal(err)
		}
		_, reason, _ := v.Verify(context.Background(), plaintext)
		if reason != "expired" {
			t.Errorf("reason = %q, want expired", reason)
		}
	})
}

func TestTokenVerifier_CacheHit(t *testing.T) {
	t.Parallel()
	v, db, plaintext, s := verifierTestSetup(t)

	// First Verify populates the cache.
	if _, reason, err := v.Verify(context.Background(), plaintext); err != nil || reason != "success" {
		t.Fatalf("first Verify: reason=%q err=%v", reason, err)
	}

	// Revoke at the DB level, but DON'T invalidate the cache. A cache
	// hit must continue to succeed for the TTL window — that's the
	// trade-off the cache exists to make. The companion test below
	// covers the post-Invalidate behaviour.
	if err := db.AuthTokens().Revoke(context.Background(), s.TokenID, "u1", "test", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	p, reason, err := v.Verify(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("second Verify err: %v", err)
	}
	if reason != "success" {
		t.Errorf("cache-hit reason = %q, want success", reason)
	}
	if p == nil {
		t.Fatal("principal nil on cache hit")
	}
}

func TestTokenVerifier_CacheInvalidateAfterRevoke(t *testing.T) {
	t.Parallel()
	v, db, plaintext, s := verifierTestSetup(t)

	if _, reason, _ := v.Verify(context.Background(), plaintext); reason != "success" {
		t.Fatal("priming Verify failed")
	}
	// Revoke + explicit Invalidate is the production path the session
	// logout / revoke handler takes.
	if err := db.AuthTokens().Revoke(context.Background(), s.TokenID, "u1", "leaked", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	v.Invalidate(s.TokenID)

	_, reason, _ := v.Verify(context.Background(), plaintext)
	if reason != "revoked" {
		t.Errorf("post-Invalidate reason = %q, want revoked", reason)
	}
}

func TestTokenVerifier_CacheHitWrongSecret(t *testing.T) {
	t.Parallel()
	v, _, plaintext, _ := verifierTestSetup(t)

	// Prime the cache with the real token.
	if _, reason, _ := v.Verify(context.Background(), plaintext); reason != "success" {
		t.Fatal("priming Verify failed")
	}

	// Construct a token with the SAME id but a tampered secret half.
	// id half = everything before the dot.
	dot := -1
	for i := len(plaintext) - 1; i >= 0; i-- {
		if plaintext[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		t.Fatal("plaintext missing dot")
	}
	tampered := plaintext[:dot+1] + "aaaaaaaaaaaaaaaaaaaa" // 20 valid b32 chars but wrong secret

	_, reason, err := v.Verify(context.Background(), tampered)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if reason == "success" {
		t.Errorf("reason = success on tampered secret with cached id (must reject)")
	}
}

func TestTokenVerifier_TouchLastUsed(t *testing.T) {
	t.Parallel()
	v, db, plaintext, s := verifierTestSetup(t)

	if _, reason, _ := v.Verify(context.Background(), plaintext); reason != "success" {
		t.Fatal("priming Verify failed")
	}
	// TouchLastUsed runs in a goroutine; allow it to land. We poll
	// rather than sleep blindly so the test stays quick when fast.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := db.AuthTokens().GetSession(context.Background(), s.TokenID)
		if err != nil {
			t.Fatal(err)
		}
		if got.LastUsedAt != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("LastUsedAt still nil 2s after Verify success")
}
