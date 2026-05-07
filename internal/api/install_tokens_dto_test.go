package api

import (
	"encoding/json"
	"testing"

	"github.com/WangYihang/Platypus/internal/storage"
)

// TestIssueInstallRequest_AcceptsPluginSpecs: the rich shape PR 4
// will start sending. JSON-decoded request struct carries
// PluginSpecs faithfully so the handler can route them into the
// enrollment service. Pinning at the DTO layer is enough — the
// downstream wire integration is exercised by the FE wizard tests
// against the running server.
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

// TestIssueInstallRequest_AcceptsLegacyIDs: the current FE shape
// keeps working unchanged.
func TestIssueInstallRequest_AcceptsLegacyIDs(t *testing.T) {
	body := []byte(`{
		"server_endpoint": "1.2.3.4:13337",
		"baseline_plugin_ids": ["sys-info", "shell"]
	}`)
	var req issueInstallRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.BaselinePluginIDs) != 2 || req.BaselinePluginIDs[0] != "sys-info" {
		t.Fatalf("BaselinePluginIDs = %v", req.BaselinePluginIDs)
	}
}

// TestChosenInstallPluginIDs_PrefersRichSpecs: when both shapes
// arrive, the rich PluginSpecs win and we project them down to
// plugin_id strings for the (PR-2-still-string-shaped) enrollment
// service call. PR 3 will plumb the rich spec end-to-end and this
// projection helper goes away.
func TestChosenInstallPluginIDs_PrefersRichSpecs(t *testing.T) {
	req := issueInstallRequest{
		BaselinePluginIDs: []string{"legacy-only"},
		PluginSpecs: []storage.PluginSpec{
			{PluginID: "rich-1", Version: "v1"},
			{PluginID: "rich-2"},
		},
	}
	got := chosenInstallPluginIDs(req)
	if len(got) != 2 || got[0] != "rich-1" || got[1] != "rich-2" {
		t.Fatalf("got = %v, want [rich-1 rich-2]", got)
	}
}

// TestChosenInstallPluginIDs_FallsBackToLegacy: when only legacy
// is supplied, it round-trips unchanged. This is the path the
// current FE exercises until PR 4.
func TestChosenInstallPluginIDs_FallsBackToLegacy(t *testing.T) {
	req := issueInstallRequest{
		BaselinePluginIDs: []string{"sys-info", "shell"},
	}
	got := chosenInstallPluginIDs(req)
	if len(got) != 2 || got[0] != "sys-info" {
		t.Fatalf("got = %v", got)
	}
}

// TestInstallListItem_DualEmitShape: serialised list-item carries
// both rich plugin_specs and the projected baseline_plugin_ids,
// so legacy and new clients each get the field they expect.
func TestInstallListItem_DualEmitShape(t *testing.T) {
	item := installListItem{
		DownloadID:        "dl_x",
		BaselinePluginIDs: []string{"a", "b"},
		PluginSpecs: []storage.PluginSpec{
			{PluginID: "a"}, {PluginID: "b"},
		},
	}
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"baseline_plugin_ids":["a","b"]`,
		`"plugin_specs":[`,
		`"plugin_id":"a"`,
	} {
		if indexLower(s, want) < 0 {
			t.Fatalf("response missing %q: %s", want, s)
		}
	}
}
