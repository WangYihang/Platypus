package storage_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedInstallToken(t *testing.T, db *storage.DB, id, projectID, userID string,
	secret []byte, ttl time.Duration) *storage.InstallDownloadToken {
	t.Helper()
	tok := &storage.InstallDownloadToken{
		DownloadID:     id,
		SecretHash:     hashBytes(secret),
		ProjectID:      projectID,
		IssuedByUser:   userID,
		IssuedAt:       time.Now().UTC(),
		ExpiresAt:      time.Now().Add(ttl).UTC(),
		ServerEndpoint: "127.0.0.1:13337",
		PATTTLSeconds:  3600,
	}
	if err := db.InstallDownloadTokens().Create(context.Background(), tok); err != nil {
		t.Fatalf("seedInstallToken: %v", err)
	}
	return tok
}

func TestInstallTokens_CreateGetStatus(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	tok := seedInstallToken(t, db, "dl_abc", proj.ID, admin.ID, []byte("s"), 5*time.Minute)
	got, err := db.InstallDownloadTokens().Get(context.Background(), tok.DownloadID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ServerEndpoint != "127.0.0.1:13337" {
		t.Fatalf("ServerEndpoint = %q", got.ServerEndpoint)
	}
	if got.Status(time.Now()) != storage.InstallDownloadPending {
		t.Fatalf("Status = %v; want pending", got.Status(time.Now()))
	}
}

// Migration 000031 added baseline_plugin_ids. Ensure round-trip works
// (slice in, slice back), with both populated and empty cases. Empty
// returns nil (vs. []string{}) by convention so callers can distinguish
// "no baseline provided" from "explicit empty baseline" — both behave
// the same downstream but the nil form keeps logs cleaner.
func TestInstallTokens_BaselinePluginIDs_Roundtrip(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	// With a baseline.
	tok := &storage.InstallDownloadToken{
		DownloadID:        "dl_with_baseline",
		SecretHash:        hashBytes([]byte("s")),
		ProjectID:         proj.ID,
		IssuedByUser:      admin.ID,
		IssuedAt:          time.Now().UTC(),
		ExpiresAt:         time.Now().Add(time.Hour).UTC(),
		ServerEndpoint:    "127.0.0.1:13337",
		PATTTLSeconds:     3600,
		BaselinePluginIDs: []string{"com.platypus.sys-info", "com.platypus.sys-listdir"},
	}
	if err := db.InstallDownloadTokens().Create(context.Background(), tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := db.InstallDownloadTokens().Get(context.Background(), tok.DownloadID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.BaselinePluginIDs) != 2 ||
		got.BaselinePluginIDs[0] != "com.platypus.sys-info" ||
		got.BaselinePluginIDs[1] != "com.platypus.sys-listdir" {
		t.Fatalf("BaselinePluginIDs = %v", got.BaselinePluginIDs)
	}

	// Without a baseline → nil round-trip.
	plain := seedInstallToken(t, db, "dl_no_baseline", proj.ID, admin.ID, []byte("s"), time.Hour)
	got2, err := db.InstallDownloadTokens().Get(context.Background(), plain.DownloadID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got2.BaselinePluginIDs != nil {
		t.Fatalf("BaselinePluginIDs = %v; want nil", got2.BaselinePluginIDs)
	}
}

// Happy path: secret matches, TryConsume records consumer metadata +
// linked PAT id, status flips to consumed, and nothing is deleted.
func TestInstallTokens_TryConsume_Success(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	secret := []byte("single-use")
	seedInstallToken(t, db, "dl_ok", proj.ID, admin.ID, secret, 5*time.Minute)
	// consumed_pat_id FKs to enrollment_tokens — seed the row so TryConsume
	// can persist the linkage on success.
	seedEnrollmentToken(t, db, "plt_minted", proj.ID, admin.ID, []byte("x"), time.Hour, 1)
	seedEnrollmentToken(t, db, "plt_other", proj.ID, admin.ID, []byte("y"), time.Hour, 1)

	got, outcome, err := db.InstallDownloadTokens().TryConsume(
		context.Background(), "dl_ok", secret, "10.0.0.1", "curl/8.0",
		"plt_minted", time.Now())
	if err != nil {
		t.Fatalf("TryConsume: %v", err)
	}
	if outcome != "success" {
		t.Fatalf("outcome = %q; want success", outcome)
	}
	if got.ConsumedPATID != "plt_minted" {
		t.Fatalf("ConsumedPATID = %q", got.ConsumedPATID)
	}
	if got.Status(time.Now()) != storage.InstallDownloadConsumed {
		t.Fatalf("Status = %v; want consumed", got.Status(time.Now()))
	}

	// Replay → already_consumed; the original row remains intact.
	again, outcome, err := db.InstallDownloadTokens().TryConsume(
		context.Background(), "dl_ok", secret, "10.0.0.2", "curl/8.0",
		"plt_other", time.Now())
	if err != nil {
		t.Fatalf("replay err: %v", err)
	}
	if outcome != "already_consumed" {
		t.Fatalf("replay outcome = %q", outcome)
	}
	if again.ConsumedPATID != "plt_minted" {
		t.Fatal("replay overwrote original ConsumedPATID")
	}
}

func TestInstallTokens_TryConsume_Classifications(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	ctx := context.Background()

	t.Run("unknown_id", func(t *testing.T) {
		_, outcome, err := db.InstallDownloadTokens().TryConsume(ctx, "dl_nope", []byte("x"), "", "", "", time.Now())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if outcome != "unknown_id" {
			t.Fatalf("outcome = %q", outcome)
		}
	})

	t.Run("invalid_secret", func(t *testing.T) {
		seedInstallToken(t, db, "dl_bad", proj.ID, admin.ID, []byte("right"), time.Minute)
		_, outcome, _ := db.InstallDownloadTokens().TryConsume(ctx, "dl_bad", []byte("wrong"), "", "", "", time.Now())
		if outcome != "invalid_secret" {
			t.Fatalf("outcome = %q", outcome)
		}
	})

	t.Run("expired", func(t *testing.T) {
		seedInstallToken(t, db, "dl_old", proj.ID, admin.ID, []byte("x"), -time.Minute)
		_, outcome, _ := db.InstallDownloadTokens().TryConsume(ctx, "dl_old", []byte("x"), "", "", "", time.Now())
		if outcome != "expired" {
			t.Fatalf("outcome = %q", outcome)
		}
	})

	t.Run("revoked", func(t *testing.T) {
		seedInstallToken(t, db, "dl_rev", proj.ID, admin.ID, []byte("x"), time.Minute)
		if err := db.InstallDownloadTokens().Revoke(ctx, "dl_rev", admin.ID, "leaked", time.Now()); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		_, outcome, _ := db.InstallDownloadTokens().TryConsume(ctx, "dl_rev", []byte("x"), "", "", "", time.Now())
		if outcome != "revoked" {
			t.Fatalf("outcome = %q", outcome)
		}
	})
}

// Two concurrent curls hitting the same install link: exactly one must
// record `success`, the other must see `already_consumed`. Mirrors the
// PAT redemption race test.
func TestInstallTokens_TryConsume_Concurrent(t *testing.T) {
	db := newTestDBFile(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	secret := []byte("race-me")
	seedInstallToken(t, db, "dl_race", proj.ID, admin.ID, secret, 5*time.Minute)
	// Seed a real PAT too — consumed_pat_id has an FK into enrollment_tokens,
	// so handing TryConsume a fake id would short-circuit with a
	// constraint failure instead of exercising the race logic.
	seedEnrollmentToken(t, db, "plt_fake", proj.ID, admin.ID, []byte("unused"), time.Hour, 1)

	const N = 8
	var wg sync.WaitGroup
	outcomes := make([]string, N)
	errs := make([]error, N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, o, err := db.InstallDownloadTokens().TryConsume(
				context.Background(), "dl_race", secret, "1.1.1.1", "ua",
				"plt_fake", time.Now())
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
			continue
		}
		switch o {
		case "success":
			successes++
		case "already_consumed":
		default:
			t.Errorf("unexpected outcome %q", o)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d; want exactly 1", successes)
	}
}

func TestInstallDownloadEvents_RecordUnknownID(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	// Scanning-attack scenario: event against a download_id that doesn't
	// exist in install_download_tokens. Must be accepted (no FK).
	err := db.InstallDownloadEvents().Record(ctx, &storage.InstallDownloadEvent{
		At: time.Now().UTC(), DownloadID: "dl_fake", ClientIP: "attacker",
		Outcome: "unknown_id",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	evts, err := db.InstallDownloadEvents().ListByDownload(ctx, "dl_fake", 10)
	if err != nil {
		t.Fatalf("ListByDownload: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("len = %d", len(evts))
	}
}
