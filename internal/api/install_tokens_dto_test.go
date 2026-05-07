package api

import (
	"encoding/json"
	"testing"

	"github.com/WangYihang/Platypus/internal/storage"
)

// TestIssueInstallRequest_AcceptsPluginSpecs: the rich PluginSpec
// shape JSON-decodes into the request struct faithfully so the
// handler can route it through to the enrollment service. PR 5
// finished the migration; baseline_plugin_ids no longer exists on
// the wire.
func TestIssueInstallRequest_AcceptsPluginSpecs(t *testing.T) {
	body := []byte(`{
		"server_endpoint": "1.2.3.4:13337",
		"plugin_specs": [
			{"plugin_id":"p1","version":"v1","schema_version":1},
			{"plugin_id":"p2"}
		]
	}`)
	var req issueInstallRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.PluginSpecs) != 2 {
		t.Fatalf("PluginSpecs len = %d, want 2", len(req.PluginSpecs))
	}
	if req.PluginSpecs[0].PluginID != "p1" || req.PluginSpecs[0].Version != "v1" {
		t.Fatalf("first spec: %+v", req.PluginSpecs[0])
	}
	if req.PluginSpecs[0].SchemaVersion != 1 {
		t.Fatalf("schema_version: %d", req.PluginSpecs[0].SchemaVersion)
	}
}

// TestInstallListItem_OnlyEmitsPluginSpecs pins the absence of the
// retired baseline_plugin_ids JSON key. The list-item shape is the
// only place a downstream consumer would scrape for legacy keys —
// pinning the absence here would catch a regression that
// re-introduced the dual emit.
func TestInstallListItem_OnlyEmitsPluginSpecs(t *testing.T) {
	item := installListItem{
		DownloadID: "dl_x",
		PluginSpecs: []storage.PluginSpec{
			{PluginID: "a"}, {PluginID: "b"},
		},
	}
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if indexLower(s, `"plugin_specs":[`) < 0 {
		t.Fatalf("expected plugin_specs in output: %s", s)
	}
	if indexLower(s, `"baseline_plugin_ids"`) >= 0 {
		t.Fatalf("legacy baseline_plugin_ids still emitted: %s", s)
	}
}
