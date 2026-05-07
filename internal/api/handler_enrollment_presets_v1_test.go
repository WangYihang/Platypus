package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func enrollmentPresetsTestSetup(t *testing.T) (*gin.Engine, *storage.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)
	h := NewEnrollmentPresetsHandler(db)

	r := gin.New()
	RegisterV1EnrollmentPresetRoutes(r, h, rbac)
	return r, db
}

// TestEnrollmentPresets_Create_AcceptsLegacyPluginIDs: the existing
// FE still sends baseline_plugin_ids: []string. PR 1 added the
// PluginSpec atom to storage but kept the wire shape; this pins
// that the legacy field continues to work end-to-end (POST → GET
// → roundtrip values present) while the FE catches up in PR 4.
func TestEnrollmentPresets_Create_AcceptsLegacyPluginIDs(t *testing.T) {
	r, db := enrollmentPresetsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	body := map[string]any{
		"name":                "linux-prod-legacy",
		"baseline_plugin_ids": []string{"sys-info", "shell"},
	}
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/enrollment-presets", tok, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var resp enrollmentPresetItem
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.BaselinePluginIDs) != 2 || resp.BaselinePluginIDs[0] != "sys-info" {
		t.Fatalf("response.baseline_plugin_ids = %v, want [sys-info, shell]", resp.BaselinePluginIDs)
	}
}

// TestEnrollmentPresets_Create_AcceptsPluginSpecs: the rich shape
// — what the FE will start sending in PR 4 — POSTs plugin_specs:
// [{plugin_id, version, granted_capabilities, config_overrides,
// schema_version}, ...]. Storage round-trips every field
// faithfully.
func TestEnrollmentPresets_Create_AcceptsPluginSpecs(t *testing.T) {
	r, db := enrollmentPresetsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	body := map[string]any{
		"name": "rich-spec",
		"plugin_specs": []map[string]any{
			{
				"plugin_id":            "syslog-forwarder",
				"version":              "1.4.0",
				"granted_capabilities": []string{"net.dial"},
				"config_overrides":     map[string]any{"destination": "udp://10.0.0.1:514"},
				"schema_version":       1,
			},
		},
	}
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/enrollment-presets", tok, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var resp enrollmentPresetItem
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.PluginSpecs) != 1 {
		t.Fatalf("PluginSpecs len = %d, want 1", len(resp.PluginSpecs))
	}
	got := resp.PluginSpecs[0]
	if got.PluginID != "syslog-forwarder" || got.Version != "1.4.0" {
		t.Fatalf("identity: %+v", got)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if len(got.GrantedCapabilities) != 1 || got.GrantedCapabilities[0] != "net.dial" {
		t.Fatalf("caps = %v", got.GrantedCapabilities)
	}
	// config_overrides round-trips as JSON bytes.
	if got.ConfigOverrides == nil ||
		!jsonContains(got.ConfigOverrides, `"destination":"udp://10.0.0.1:514"`) {
		t.Fatalf("config_overrides = %s", got.ConfigOverrides)
	}
}

// TestEnrollmentPresets_DualEmit_ResponseCarriesBothShapes: while
// the FE migrates, every list / get response must include BOTH
// the rich plugin_specs and the projected baseline_plugin_ids
// shapes so legacy consumers keep working alongside the new ones.
// PR 4 drops baseline_plugin_ids; until then the dual-emit is
// load-bearing.
func TestEnrollmentPresets_DualEmit_ResponseCarriesBothShapes(t *testing.T) {
	r, db := enrollmentPresetsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	body := map[string]any{
		"name": "dual",
		"plugin_specs": []map[string]any{
			{"plugin_id": "a"}, {"plugin_id": "b"},
		},
	}
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/enrollment-presets", tok, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d", w.Code)
	}
	var resp enrollmentPresetItem
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.PluginSpecs) != 2 || len(resp.BaselinePluginIDs) != 2 {
		t.Fatalf("dual emit failed: specs=%d, ids=%d", len(resp.PluginSpecs), len(resp.BaselinePluginIDs))
	}
	if resp.BaselinePluginIDs[0] != "a" || resp.BaselinePluginIDs[1] != "b" {
		t.Fatalf("projection from specs to ids drifted: %v", resp.BaselinePluginIDs)
	}
}

// TestEnrollmentPresets_PluginSpecsWinsOverLegacy: when both
// fields are supplied (a transitional FE that's mid-migration, or
// a confused client), the rich shape wins. This is the "no
// silent data loss" property: the client that knows about
// PluginSpec gets the storage shape it expects.
func TestEnrollmentPresets_PluginSpecsWinsOverLegacy(t *testing.T) {
	r, db := enrollmentPresetsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	body := map[string]any{
		"name":                "both",
		"baseline_plugin_ids": []string{"legacy-only"},
		"plugin_specs": []map[string]any{
			{"plugin_id": "rich-1", "version": "v1"},
		},
	}
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/enrollment-presets", tok, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d", w.Code)
	}
	var resp enrollmentPresetItem
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.PluginSpecs) != 1 || resp.PluginSpecs[0].PluginID != "rich-1" {
		t.Fatalf("plugin_specs lost: %+v", resp.PluginSpecs)
	}
	if resp.PluginSpecs[0].Version != "v1" {
		t.Fatalf("rich field dropped: %+v", resp.PluginSpecs[0])
	}
}

// jsonContains is a quick "does this JSON blob mention this
// substring after key normalisation" check. We round-trip through
// encoding/json so cosmetic whitespace doesn't make the assertion
// brittle.
func jsonContains(b []byte, want string) bool {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return false
	}
	canonical, _ := json.Marshal(v)
	return indexLower(string(canonical), want) >= 0
}
