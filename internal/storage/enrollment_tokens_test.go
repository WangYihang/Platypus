package storage_test

import (
	"context"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedEnrollmentToken(t *testing.T, db *storage.DB, tokenID string, projectID, userID string,
	secret []byte, expiresIn time.Duration, maxUses int) *storage.EnrollmentToken {
	t.Helper()
	hash := sha256.Sum256(secret)
	p := &storage.EnrollmentToken{
		TokenID:      tokenID,
		SecretHash:   hash[:],
		ProjectID:    projectID,
		IssuedByUser: userID,
		IssuedAt:     time.Now().UTC(),
		ExpiresAt:    time.Now().Add(expiresIn).UTC(),
		MaxUses:      maxUses,
	}
	if err := db.EnrollmentTokens().Create(context.Background(), p); err != nil {
		t.Fatalf("seedEnrollmentToken: %v", err)
	}
	return p
}

func TestEnrollmentTokens_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	p := seedEnrollmentToken(t, db, "plt_abc", proj.ID, admin.ID, []byte("shh"), time.Hour, 1)

	got, err := db.EnrollmentTokens().Get(context.Background(), p.TokenID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Uses != 0 || got.MaxUses != 1 {
		t.Fatalf("Uses/MaxUses = %d/%d; want 0/1", got.Uses, got.MaxUses)
	}
	if got.Status(time.Now()) != storage.EnrollmentStatusPending {
		t.Fatalf("Status = %v; want pending", got.Status(time.Now()))
	}
}

func TestEnrollmentTokens_TryConsume_Success(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	secret := []byte("correct-horse")
	seedEnrollmentToken(t, db, "plt_good", proj.ID, admin.ID, secret, time.Hour, 1)

	got, outcome, err := db.EnrollmentTokens().TryConsume(
		context.Background(), "plt_good", secret, "machine-x", time.Now())
	if err != nil {
		t.Fatalf("TryConsume: %v", err)
	}
	if outcome != "success" {
		t.Fatalf("outcome = %q; want success", outcome)
	}
	if got.Uses != 1 {
		t.Fatalf("Uses = %d; want 1", got.Uses)
	}
	if got.Status(time.Now()) != storage.EnrollmentStatusConsumed {
		t.Fatalf("Status = %v; want consumed", got.Status(time.Now()))
	}
}

func TestEnrollmentTokens_TryConsume_Classifications(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	ctx := context.Background()

	t.Run("unknown_token", func(t *testing.T) {
		_, outcome, err := db.EnrollmentTokens().TryConsume(ctx, "plt_nope", []byte("x"), "", time.Now())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if outcome != "unknown_token" {
			t.Fatalf("outcome = %q", outcome)
		}
	})

	t.Run("invalid_secret", func(t *testing.T) {
		seedEnrollmentToken(t, db, "plt_bad_secret", proj.ID, admin.ID, []byte("right"), time.Hour, 1)
		_, outcome, err := db.EnrollmentTokens().TryConsume(ctx, "plt_bad_secret", []byte("wrong"), "", time.Now())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if outcome != "invalid_secret" {
			t.Fatalf("outcome = %q; want invalid_secret", outcome)
		}
	})

	t.Run("expired", func(t *testing.T) {
		seedEnrollmentToken(t, db, "plt_old", proj.ID, admin.ID, []byte("x"), -time.Minute, 1)
		_, outcome, err := db.EnrollmentTokens().TryConsume(ctx, "plt_old", []byte("x"), "", time.Now())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if outcome != "expired" {
			t.Fatalf("outcome = %q; want expired", outcome)
		}
	})

	t.Run("revoked", func(t *testing.T) {
		seedEnrollmentToken(t, db, "plt_rev", proj.ID, admin.ID, []byte("x"), time.Hour, 1)
		if err := db.EnrollmentTokens().Revoke(ctx, "plt_rev", admin.ID, "leaked", time.Now()); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		_, outcome, err := db.EnrollmentTokens().TryConsume(ctx, "plt_rev", []byte("x"), "", time.Now())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if outcome != "revoked" {
			t.Fatalf("outcome = %q; want revoked", outcome)
		}
	})

	t.Run("max_uses_reached", func(t *testing.T) {
		seedEnrollmentToken(t, db, "plt_used", proj.ID, admin.ID, []byte("x"), time.Hour, 1)
		// consume once
		_, outcome, _ := db.EnrollmentTokens().TryConsume(ctx, "plt_used", []byte("x"), "", time.Now())
		if outcome != "success" {
			t.Fatalf("first consume outcome = %q", outcome)
		}
		_, outcome, _ = db.EnrollmentTokens().TryConsume(ctx, "plt_used", []byte("x"), "", time.Now())
		if outcome != "max_uses_reached" {
			t.Fatalf("second outcome = %q; want max_uses_reached", outcome)
		}
	})

	t.Run("binding_machine_mismatch", func(t *testing.T) {
		p := &storage.EnrollmentToken{
			TokenID:          "plt_bound",
			SecretHash:       hashBytes([]byte("x")),
			ProjectID:        proj.ID,
			IssuedByUser:     admin.ID,
			IssuedAt:         time.Now().UTC(),
			ExpiresAt:        time.Now().Add(time.Hour).UTC(),
			MaxUses:          1,
			BindingMachineID: "host-abc",
		}
		if err := db.EnrollmentTokens().Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}
		_, outcome, _ := db.EnrollmentTokens().TryConsume(ctx, "plt_bound", []byte("x"), "host-xyz", time.Now())
		if outcome != "binding_machine_mismatch" {
			t.Fatalf("outcome = %q", outcome)
		}
		// matching machine_id works
		_, outcome, _ = db.EnrollmentTokens().TryConsume(ctx, "plt_bound", []byte("x"), "host-abc", time.Now())
		if outcome != "success" {
			t.Fatalf("matched outcome = %q; want success", outcome)
		}
	})
}

// Concurrency safety: N parallel goroutines racing on a single-use
// enrollment token; exactly one must succeed and the others must get
// max_uses_reached. Uses a temp file DB because modernc.org/sqlite's
// ":memory:" driver creates one database per connection — the shared
// schema only materialises when all queries go through the same
// underlying *sqlite.conn.
func TestEnrollmentTokens_TryConsume_Concurrent(t *testing.T) {
	db := newTestDBFile(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	secret := []byte("single-use-secret")
	seedEnrollmentToken(t, db, "plt_race", proj.ID, admin.ID, secret, time.Hour, 1)

	const N = 16
	var wg sync.WaitGroup
	outcomes := make([]string, N)
	errs := make([]error, N)
	start := make(chan struct{})

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, o, err := db.EnrollmentTokens().TryConsume(
				context.Background(), "plt_race", secret, "machine", time.Now())
			outcomes[idx] = o
			errs[idx] = err
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for i, o := range outcomes {
		if errs[i] != nil {
			t.Errorf("goroutine %d err: %v", i, errs[i])
		}
		switch o {
		case "success":
			successes++
		case "max_uses_reached":
			// expected loser
		default:
			t.Errorf("goroutine %d unexpected outcome: %q", i, o)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one success; got %d", successes)
	}
}

func TestEnrollmentTokens_Revoke_NotFound(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	err := db.EnrollmentTokens().Revoke(context.Background(), "plt_ghost", admin.ID, "none", time.Now())
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func hashBytes(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
