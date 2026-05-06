package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBulkExec_Success: three agents echo back command + args via
// the stub handler; result rows surface stdout/stderr/exit_code.
func TestBulkExec_Success(t *testing.T) {
	agentIDs := []string{"a1", "a2", "a3"}
	var calls sync.Map
	env := setupBulkRPC(t, agentIDs, func(id string, req *v2pb.RpcRequest) *v2pb.RpcResponse {
		calls.Store(id, true)
		ex := req.GetExec()
		if ex == nil {
			return &v2pb.RpcResponse{Error: "expected exec"}
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{
			Exec: &v2pb.ExecResponse{
				ExitCode: 0,
				Stdout:   []byte(ex.Command + "@" + id),
			},
		}}
	})

	resp := env.postBulk(t, "/exec", map[string]any{
		"agent_ids":  agentIDs,
		"command":    "uname",
		"args":       []string{"-a"},
		"timeout_ms": 5000,
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
			ExitCode int32  `json:"exit_code"`
			Stdout   []byte `json:"stdout"`
			Stderr   []byte `json:"stderr"`
			Error    string `json:"error"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(body.Results))
	}
	for i, r := range body.Results {
		if r.AgentID != agentIDs[i] {
			t.Errorf("results[%d].agent_id = %q, want %q", i, r.AgentID, agentIDs[i])
		}
		if !r.Ok || r.Error != "" {
			t.Errorf("results[%d] = %+v", i, r)
		}
		want := "uname@" + agentIDs[i]
		if string(r.Stdout) != want {
			t.Errorf("results[%d].stdout = %q, want %q", i, r.Stdout, want)
		}
	}
}

// TestBulkExec_NonZeroExit captures non-zero exit_code on a row
// without flagging Ok=false. The "agent ran the command and got
// exit_code=N" case is distinct from "agent transport failed" —
// the former is still a successful round-trip with structured
// failure info.
func TestBulkExec_NonZeroExit(t *testing.T) {
	env := setupBulkRPC(t, []string{"a1"}, func(_ string, _ *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{
			Exec: &v2pb.ExecResponse{
				ExitCode: 1,
				Stderr:   []byte("permission denied"),
			},
		}}
	})
	resp := env.postBulk(t, "/exec", map[string]any{
		"agent_ids": []string{"a1"},
		"command":   "false",
	})
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	defer resp.Body.Close()
	var body struct {
		Results []struct {
			Ok       bool   `json:"ok"`
			ExitCode int32  `json:"exit_code"`
			Stderr   []byte `json:"stderr"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Results) != 1 {
		t.Fatalf("results = %d", len(body.Results))
	}
	r := body.Results[0]
	if !r.Ok {
		t.Errorf("non-zero exit must still be ok=true (transport succeeded): %+v", r)
	}
	if r.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", r.ExitCode)
	}
	if string(r.Stderr) != "permission denied" {
		t.Errorf("stderr = %q", r.Stderr)
	}
}

// TestBulkExec_RejectsEmptyAgentIDs: 400 on missing list.
func TestBulkExec_RejectsEmptyAgentIDs(t *testing.T) {
	env := setupBulkRPC(t, []string{"a1"}, func(_ string, _ *v2pb.RpcRequest) *v2pb.RpcResponse {
		t.Fatalf("dispatcher must not be called")
		return nil
	})
	resp := env.postBulk(t, "/exec", map[string]any{
		"agent_ids": []string{},
		"command":   "echo",
	})
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestBulkExec_RequiresCommand: 400 on missing command.
func TestBulkExec_RequiresCommand(t *testing.T) {
	env := setupBulkRPC(t, []string{"a1"}, func(_ string, _ *v2pb.RpcRequest) *v2pb.RpcResponse {
		t.Fatalf("dispatcher must not be called")
		return nil
	})
	resp := env.postBulk(t, "/exec", map[string]any{
		"agent_ids": []string{"a1"},
	})
	if resp.StatusCode != 400 {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d body=%s, want 400", resp.StatusCode, b)
	}
}

// TestBulkExec_OfflineAgent: an unknown agent in the request →
// 403 (project-membership guard fires before any dispatch).
func TestBulkExec_OfflineAgent(t *testing.T) {
	env := setupBulkRPC(t, []string{"on1"}, func(_ string, _ *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{
			Exec: &v2pb.ExecResponse{ExitCode: 0},
		}}
	})
	resp := env.postBulk(t, "/exec", map[string]any{
		"agent_ids": []string{"on1", "ghost"},
		"command":   "echo",
	})
	if resp.StatusCode != 403 && resp.StatusCode != 404 {
		t.Errorf("status = %d, want 403/404", resp.StatusCode)
	}
}

// Quick assertion that http.MethodPost is used (sanity for
// route-registration regressions).
var _ = http.MethodPost
