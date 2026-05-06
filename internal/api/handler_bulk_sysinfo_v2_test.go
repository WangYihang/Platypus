package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBulkSysInfo_Success: per-agent sysinfo response surfaces
// hostname / os / uptime / load fields in one shot.
func TestBulkSysInfo_Success(t *testing.T) {
	agentIDs := []string{"a1", "a2"}
	env := setupBulkRPC(t, agentIDs, func(id string, req *v2pb.RpcRequest) *v2pb.RpcResponse {
		if req.GetSysInfo() == nil {
			return &v2pb.RpcResponse{Error: "expected sys_info"}
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_SysInfo{
			SysInfo: &v2pb.SysInfoResponse{
				Hostname:      "host-" + id,
				Os:            "linux",
				KernelVersion: "6.8",
				NumCpu:        8,
				MemTotal:      16 * 1024 * 1024 * 1024,
				MemUsed:       4 * 1024 * 1024 * 1024,
				UptimeSeconds: 12345,
				Load1:         0.5,
			},
		}}
	})

	resp := env.postBulk(t, "/sys_info", map[string]any{
		"agent_ids": agentIDs,
	})
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	defer resp.Body.Close()

	var body struct {
		Results []struct {
			AgentID  string `json:"agent_id"`
			Ok       bool   `json:"ok"`
			Hostname string `json:"hostname"`
			Os       string `json:"os"`
			MemTotal uint64 `json:"mem_total"`
			Load1    float64 `json:"load1"`
			Error    string `json:"error"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(body.Results))
	}
	for i, r := range body.Results {
		if r.AgentID != agentIDs[i] {
			t.Errorf("[%d].agent_id = %q, want %q", i, r.AgentID, agentIDs[i])
		}
		if !r.Ok || r.Error != "" {
			t.Errorf("[%d] = %+v, want ok+no err", i, r)
		}
		want := "host-" + agentIDs[i]
		if r.Hostname != want {
			t.Errorf("[%d].hostname = %q, want %q", i, r.Hostname, want)
		}
		if r.Os != "linux" {
			t.Errorf("[%d].os = %q", i, r.Os)
		}
		if r.MemTotal != 16*1024*1024*1024 {
			t.Errorf("[%d].mem_total = %d", i, r.MemTotal)
		}
		if r.Load1 != 0.5 {
			t.Errorf("[%d].load1 = %v", i, r.Load1)
		}
	}
}

// TestBulkSysInfo_RejectsEmptyAgentIDs: 400 on empty list.
func TestBulkSysInfo_RejectsEmptyAgentIDs(t *testing.T) {
	env := setupBulkRPC(t, []string{"a1"}, func(_ string, _ *v2pb.RpcRequest) *v2pb.RpcResponse {
		t.Fatalf("must not dispatch")
		return nil
	})
	resp := env.postBulk(t, "/sys_info", map[string]any{
		"agent_ids": []string{},
	})
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestBulkSysInfo_OfflineAgentRejectedByPivot: ghost id → 403.
func TestBulkSysInfo_OfflineAgentRejectedByPivot(t *testing.T) {
	env := setupBulkRPC(t, []string{"on1"}, func(_ string, _ *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_SysInfo{
			SysInfo: &v2pb.SysInfoResponse{},
		}}
	})
	resp := env.postBulk(t, "/sys_info", map[string]any{
		"agent_ids": []string{"on1", "ghost"},
	})
	if resp.StatusCode != 403 && resp.StatusCode != 404 {
		t.Errorf("status = %d, want 403/404", resp.StatusCode)
	}
}

var _ = http.MethodPost
