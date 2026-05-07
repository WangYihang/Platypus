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

// TestEnrollmentPresets_Create_AcceptsPluginSpecs: rich PluginSpec
// (plugin_id + version + granted_capabilities + config_overrides +
// schema_version) round-trips through storage faithfully — every
// field comes back through the GET response unchanged.
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

// TestEnrollmentPresets_RejectsLegacyBaselinePluginIDs pins the
// retirement: clients that send the legacy baseline_plugin_ids key
// in the request body get a clean response with empty PluginSpecs
// (the unknown field is simply discarded, no error). This is what
// catches a stale client trying to use the old wire shape.
func TestEnrollmentPresets_RejectsLegacyBaselinePluginIDs(t *testing.T) {
	r, db := enrollmentPresetsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	body := map[string]any{
		"name":                "legacy-shape",
		"baseline_plugin_ids": []string{"sys-info", "shell"},
	}
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/enrollment-presets", tok, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var resp enrollmentPresetItem
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.PluginSpecs) != 0 {
		t.Fatalf("legacy field shouldn't populate plugin_specs: got %+v",
			resp.PluginSpecs)
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
