package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func intPtr(v int) *int { return &v }

func newPresetID(t *testing.T) string {
	t.Helper()
	id, err := storage.NewPresetID()
	if err != nil {
		t.Fatalf("NewPresetID: %v", err)
	}
	return id
}

func TestEnrollmentPresets_CreateGetList(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	now := time.Now().UTC()
	p := &storage.EnrollmentPreset{
		PresetID:            newPresetID(t),
		ProjectID:           proj.ID,
		Name:                "linux-prod",
		Description:         "5min TTL, single use",
		ServerEndpoint:      "203.0.113.5:13337",
		TargetOS:            "linux",
		TargetArch:          "amd64",
		TTLSeconds:          intPtr(300),
		PATMaxUses:          intPtr(1),
		AutoApprove:         false,
		SkipTLSVerification: true,
		PluginSpecs: []storage.PluginSpec{
			{PluginID: "sys-info"},
			{PluginID: "shell"},
		},
		PATDescription:      "Linux fleet baseline",
		CreatedByUser:       admin.ID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := db.EnrollmentPresets().Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := db.EnrollmentPresets().Get(context.Background(), p.PresetID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "linux-prod" || got.TargetOS != "linux" || got.TargetArch != "amd64" {
		t.Fatalf("identity: %+v", got)
	}
	if got.TTLSeconds == nil || *got.TTLSeconds != 300 {
		t.Fatalf("TTLSeconds = %v; want *300", got.TTLSeconds)
	}
	if !got.SkipTLSVerification || got.AutoApprove {
		t.Fatalf("flags: skipTLS=%v autoApprove=%v", got.SkipTLSVerification, got.AutoApprove)
	}
	if len(got.PluginSpecs) != 2 || got.PluginSpecs[0].PluginID != "sys-info" {
		t.Fatalf("plugins = %v", got.PluginSpecs)
	}

	// Listing returns at least our row.
	list, err := db.EnrollmentPresets().ListByProject(context.Background(), proj.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(list) != 1 || list[0].PresetID != p.PresetID {
		t.Fatalf("list = %+v", list)
	}
}

func TestEnrollmentPresets_GetMissing(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.EnrollmentPresets().Get(context.Background(), "epr_nope"); err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func TestEnrollmentPresets_Update(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	now := time.Now().UTC()
	p := &storage.EnrollmentPreset{
		PresetID:    newPresetID(t),
		ProjectID:   proj.ID,
		Name:        "win",
		TargetOS:    "windows",
		TargetArch:  "amd64",
		TTLSeconds:  intPtr(60),
		PATMaxUses:  intPtr(1),
		AutoApprove: false,
		SkipTLSVerification: true,
		CreatedByUser: admin.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.EnrollmentPresets().Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	p.Name = "win-prod"
	p.TTLSeconds = intPtr(900)
	p.AutoApprove = true
	p.PluginSpecs = []storage.PluginSpec{{PluginID: "shell"}}
	p.UpdatedAt = now.Add(time.Minute)
	if err := db.EnrollmentPresets().Update(context.Background(), p); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := db.EnrollmentPresets().Get(context.Background(), p.PresetID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "win-prod" || *got.TTLSeconds != 900 || !got.AutoApprove {
		t.Fatalf("post-update: %+v", got)
	}
	if len(got.PluginSpecs) != 1 || got.PluginSpecs[0].PluginID != "shell" {
		t.Fatalf("plugins post-update = %v", got.PluginSpecs)
	}
}

func TestEnrollmentPresets_UpdateMissing(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	p := &storage.EnrollmentPreset{
		PresetID:  "epr_ghost",
		ProjectID: "p1",
		Name:      "x",
		UpdatedAt: now,
	}
	if err := db.EnrollmentPresets().Update(context.Background(), p); err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func TestEnrollmentPresets_Delete(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	p := &storage.EnrollmentPreset{
		PresetID:  newPresetID(t),
		ProjectID: proj.ID,
		Name:      "tmp",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.EnrollmentPresets().Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := db.EnrollmentPresets().Delete(context.Background(), p.PresetID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.EnrollmentPresets().Get(context.Background(), p.PresetID); err != storage.ErrNotFound {
		t.Fatalf("Get after delete err = %v; want ErrNotFound", err)
	}
	if err := db.EnrollmentPresets().Delete(context.Background(), p.PresetID); err != storage.ErrNotFound {
		t.Fatalf("second Delete err = %v; want ErrNotFound", err)
	}
}

func TestEnrollmentPresets_SeedSystemPresets_Idempotent(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	supported := []storage.SeedPlatform{
		{OS: "linux", Arch: "amd64"},
		{OS: "windows", Arch: "amd64"},
		{OS: "darwin", Arch: "arm64"},
	}
	now := time.Now().UTC()

	first, err := db.EnrollmentPresets().SeedSystemPresets(context.Background(), proj.ID, supported, now, admin.ID)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("len(first) = %d; want 3", len(first))
	}
	for _, p := range first {
		if !p.IsSeed {
			t.Fatalf("preset %q not flagged as seed", p.Name)
		}
		if !p.SkipTLSVerification {
			t.Fatalf("preset %q SkipTLSVerification=false; want true", p.Name)
		}
		if p.AutoApprove {
			t.Fatalf("preset %q AutoApprove=true; want conservative false", p.Name)
		}
		if p.TTLSeconds == nil || *p.TTLSeconds != 300 {
			t.Fatalf("preset %q TTL = %v; want 300", p.Name, p.TTLSeconds)
		}
	}

	// Re-running with the same input doesn't duplicate rows — INSERT
	// OR IGNORE collapses into a no-op.
	second, err := db.EnrollmentPresets().SeedSystemPresets(context.Background(), proj.ID, supported, now.Add(time.Minute), admin.ID)
	if err != nil {
		t.Fatalf("Seed (second): %v", err)
	}
	if len(second) != 3 {
		t.Fatalf("len(second) = %d; want 3 (idempotent)", len(second))
	}
}

func TestEnrollmentPresets_SeedSystemPresets_FilterUnsupported(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	// Manifest published, but only linux/amd64 — windows + macOS get skipped.
	supported := []storage.SeedPlatform{{OS: "linux", Arch: "amd64"}}
	out, err := db.EnrollmentPresets().SeedSystemPresets(
		context.Background(), proj.ID, supported, time.Now().UTC(), admin.ID,
	)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(out) != 1 || out[0].TargetOS != "linux" {
		t.Fatalf("filtered seed = %+v; want only linux/amd64", out)
	}
}

func TestEnrollmentPresets_SeedSystemPresets_NoManifest(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	// supported=nil means "manifest empty / not published". Convention is
	// to seed all three anyway — the install endpoint still works in
	// auto-detect mode, so the cards remain useful.
	out, err := db.EnrollmentPresets().SeedSystemPresets(
		context.Background(), proj.ID, nil, time.Now().UTC(), admin.ID,
	)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len = %d; want 3", len(out))
	}
}

func TestEnrollmentPresets_Create_DuplicateName(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	now := time.Now().UTC()
	mk := func(id, name string) *storage.EnrollmentPreset {
		return &storage.EnrollmentPreset{
			PresetID:  id,
			ProjectID: proj.ID,
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	if err := db.EnrollmentPresets().Create(context.Background(), mk(newPresetID(t), "dup")); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// Second insert with the same (project_id, name) must fail —
	// callers rely on this for seed idempotency.
	if err := db.EnrollmentPresets().Create(context.Background(), mk(newPresetID(t), "dup")); err == nil {
		t.Fatalf("expected UNIQUE error on duplicate (project_id, name)")
	}
}
