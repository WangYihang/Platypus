package settings_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/settings"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	"github.com/WangYihang/Platypus/internal/utils/config"
)

func newTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedAdminUser(t *testing.T, db *storage.DB, id string) {
	t.Helper()
	u := &user.User{
		ID:           id,
		Username:     id,
		PasswordHash: "hash",
		Role:         user.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

// ------------------- resolution chain tests -------------------

func TestRegistry_DefaultWhenNoConfigOrDB(t *testing.T) {
	db := newTestDB(t)
	reg := settings.New(db, nil)

	if got := reg.AccessTokenTTL(); got != settings.DefaultAccessTokenTTL {
		t.Errorf("AccessTokenTTL default = %v, want %v", got, settings.DefaultAccessTokenTTL)
	}
	if got := reg.DistributorChannel(); got != settings.DefaultDistributorChannel {
		t.Errorf("DistributorChannel default = %q, want %q", got, settings.DefaultDistributorChannel)
	}
	if got := reg.MeshDiscoveryLAN(); got != settings.DefaultMeshDiscoveryLAN {
		t.Errorf("MeshDiscoveryLAN default = %v, want %v", got, settings.DefaultMeshDiscoveryLAN)
	}
	if got := reg.DistributorPresignedTTL(); got != settings.DefaultPresignedTTL {
		t.Errorf("DistributorPresignedTTL default = %v, want %v", got, settings.DefaultPresignedTTL)
	}
}

func TestRegistry_YAMLFallbackWinsOverDefault(t *testing.T) {
	db := newTestDB(t)
	cfg := &config.Config{
		RESTful: config.RESTfulConfig{
			AccessExpireTime:  123,
			RefreshExpireTime: 456,
		},
		Distributor: config.DistributorConfig{
			Channel:      "yaml-channel",
			PresignedTTL: "2m",
		},
		Mesh: config.MeshConfig{
			DiscoveryLAN:      false,
			DiscoveryInterval: 77,
		},
	}
	reg := settings.New(db, cfg)

	if got := reg.AccessTokenTTL(); got != 123*time.Second {
		t.Errorf("AccessTokenTTL yaml = %v, want 123s", got)
	}
	if got := reg.RefreshTokenTTL(); got != 456*time.Second {
		t.Errorf("RefreshTokenTTL yaml = %v, want 456s", got)
	}
	if got := reg.DistributorChannel(); got != "yaml-channel" {
		t.Errorf("DistributorChannel yaml = %q", got)
	}
	if got := reg.DistributorPresignedTTL(); got != 2*time.Minute {
		t.Errorf("DistributorPresignedTTL yaml = %v", got)
	}
	if reg.MeshDiscoveryLAN() {
		t.Errorf("MeshDiscoveryLAN yaml = true, want false")
	}
	if got := reg.MeshDiscoveryInterval(); got != 77*time.Second {
		t.Errorf("MeshDiscoveryInterval yaml = %v, want 77s", got)
	}
}

func TestRegistry_DBOverrideWinsOverYAML(t *testing.T) {
	db := newTestDB(t)
	seedAdminUser(t, db, "admin")
	cfg := &config.Config{
		RESTful: config.RESTfulConfig{AccessExpireTime: 999},
	}
	reg := settings.New(db, cfg)

	ctx := context.Background()
	if err := reg.Set(ctx, settings.KeyAuthAccessTokenTTL, "60", "admin"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := reg.AccessTokenTTL(); got != 60*time.Second {
		t.Errorf("AccessTokenTTL after Set = %v, want 60s", got)
	}
}

func TestRegistry_ResetRevertsToYAML(t *testing.T) {
	db := newTestDB(t)
	seedAdminUser(t, db, "admin")
	cfg := &config.Config{
		RESTful: config.RESTfulConfig{AccessExpireTime: 999},
	}
	reg := settings.New(db, cfg)

	ctx := context.Background()
	if err := reg.Set(ctx, settings.KeyAuthAccessTokenTTL, "60", "admin"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if reg.AccessTokenTTL() != 60*time.Second {
		t.Fatal("precondition: override not applied")
	}
	if err := reg.Reset(ctx, settings.KeyAuthAccessTokenTTL, "admin"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got := reg.AccessTokenTTL(); got != 999*time.Second {
		t.Errorf("AccessTokenTTL after Reset = %v, want 999s (yaml)", got)
	}
}

// ------------------- cache tests -------------------

func TestRegistry_SetInvalidatesCache(t *testing.T) {
	db := newTestDB(t)
	seedAdminUser(t, db, "admin")
	reg := settings.New(db, nil)

	// First read populates cache with default.
	before := reg.AccessTokenTTL()
	if before != settings.DefaultAccessTokenTTL {
		t.Fatalf("precondition: %v", before)
	}
	// Set should invalidate; next read sees DB value.
	if err := reg.Set(context.Background(), settings.KeyAuthAccessTokenTTL, "30", "admin"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := reg.AccessTokenTTL(); got != 30*time.Second {
		t.Errorf("post-Set = %v, want 30s (cache not invalidated?)", got)
	}
}

// ------------------- write validation -------------------

func TestRegistry_SetRejectsUnknownKey(t *testing.T) {
	db := newTestDB(t)
	reg := settings.New(db, nil)
	err := reg.Set(context.Background(), "not.a.real.key", "1", "admin")
	if !errors.Is(err, settings.ErrUnknownKey) {
		t.Fatalf("err = %v, want ErrUnknownKey", err)
	}
}

func TestRegistry_SetRejectsWrongType(t *testing.T) {
	db := newTestDB(t)
	reg := settings.New(db, nil)

	cases := []struct {
		key string
		raw string
	}{
		{settings.KeyAuthAccessTokenTTL, "\"fifteen\""}, // string where duration expected
		{settings.KeyMeshDiscoveryLAN, "42"},            // number where bool expected
		{settings.KeyDistributorChannel, "true"},        // bool where string expected
	}
	for _, c := range cases {
		err := reg.Set(context.Background(), c.key, c.raw, "admin")
		if !errors.Is(err, settings.ErrBadValue) {
			t.Errorf("Set(%s,%s) err = %v, want ErrBadValue", c.key, c.raw, err)
		}
	}
}

func TestRegistry_SetRejectsNegativeDuration(t *testing.T) {
	db := newTestDB(t)
	reg := settings.New(db, nil)
	err := reg.Set(context.Background(), settings.KeyAuthAccessTokenTTL, "-10", "admin")
	if !errors.Is(err, settings.ErrBadValue) {
		t.Fatalf("err = %v, want ErrBadValue", err)
	}
}

// ------------------- describe tests -------------------

func TestRegistry_DescribeAll(t *testing.T) {
	db := newTestDB(t)
	seedAdminUser(t, db, "admin")
	cfg := &config.Config{
		RESTful: config.RESTfulConfig{AccessExpireTime: 200},
	}
	reg := settings.New(db, cfg)

	// Override one key via Set so we can assert source="db" for it.
	if err := reg.Set(context.Background(), settings.KeyDistributorChannel, "\"beta\"", "admin"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	descs, err := reg.DescribeAll(context.Background())
	if err != nil {
		t.Fatalf("DescribeAll: %v", err)
	}
	if len(descs) != 6 {
		t.Fatalf("len = %d, want 6", len(descs))
	}

	byKey := map[string]settings.SettingDescriptor{}
	for _, d := range descs {
		byKey[d.Key] = d
	}

	access := byKey[settings.KeyAuthAccessTokenTTL]
	if access.Source != "yaml" {
		t.Errorf("access source = %q, want yaml", access.Source)
	}
	if access.YAMLValue == nil {
		t.Errorf("access yaml value missing")
	}

	channel := byKey[settings.KeyDistributorChannel]
	if channel.Source != "db" {
		t.Errorf("channel source = %q, want db", channel.Source)
	}
	if channel.DBValue != "beta" {
		t.Errorf("channel db = %v, want beta", channel.DBValue)
	}
	if channel.Effective != "beta" {
		t.Errorf("channel effective = %v, want beta", channel.Effective)
	}

	// A key with no YAML and no DB should report source="default".
	refresh := byKey[settings.KeyAuthRefreshTokenTTL]
	if refresh.Source != "default" {
		t.Errorf("refresh source = %q, want default", refresh.Source)
	}
}
