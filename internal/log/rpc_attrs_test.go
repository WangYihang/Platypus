package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// renderJSON pipes a single Info call through a JSONHandler so the
// table-driven cases can assert on the exact rendered shape — flat
// slog.Group inspection is awkward, JSON is what operators actually
// see in their log pipeline.
func renderJSON(t *testing.T, attrs ...any) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("evt", attrs...)
	out := map[string]any{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, buf.String())
	}
	return out
}

func TestRPCMethodName(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"exec request", &v2pb.RpcRequest_Exec{}, "exec"},
		{"exec response", &v2pb.RpcResponse_Exec{}, "exec"},
		{"list_dir request", &v2pb.RpcRequest_ListDir{}, "list_dir"},
		{"process_list request", &v2pb.RpcRequest_ProcessList{}, "process_list"},
		{"nil", nil, ""},
		{"unknown", "not a payload", "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RPCMethodName(c.in); got != c.want {
				t.Fatalf("RPCMethodName(%T) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRPCRequestAttr_ListDir(t *testing.T) {
	out := renderJSON(t, RPCRequestAttr(&v2pb.RpcRequest_ListDir{ListDir: &v2pb.ListDirRequest{Path: "/var/log"}}))
	req, ok := out["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested request group, got %T (%v)", out["request"], out)
	}
	if got := req["path"]; got != "/var/log" {
		t.Fatalf("request.path = %v; want /var/log", got)
	}
}

func TestRPCRequestAttr_ExecHidesArgsAndEnv(t *testing.T) {
	// `command` is logged verbatim, `args` and `env` only by count,
	// so secrets in argv / env vars don't leak into the log stream.
	out := renderJSON(t, RPCRequestAttr(&v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{
		Command:   "psql",
		Args:      []string{"--password=hunter2", "db"},
		Env:       map[string]string{"AWS_SECRET_ACCESS_KEY": "supersecret"},
		Cwd:       "/srv",
		TimeoutMs: 5000,
	}}))
	req := out["request"].(map[string]any)
	if got := req["command"]; got != "psql" {
		t.Errorf("command = %v; want psql", got)
	}
	if got := req["arg_count"]; got.(float64) != 2 {
		t.Errorf("arg_count = %v; want 2", got)
	}
	if got := req["env_count"]; got.(float64) != 1 {
		t.Errorf("env_count = %v; want 1", got)
	}
	for _, leaked := range []string{"hunter2", "supersecret", "AWS_SECRET_ACCESS_KEY"} {
		if strings.Contains(strings.ToLower(must(json.Marshal(out))), strings.ToLower(leaked)) {
			t.Errorf("rendered log line contains sensitive substring %q: %s", leaked, must(json.Marshal(out)))
		}
	}
}

func TestRPCRequestAttr_ProcessList(t *testing.T) {
	out := renderJSON(t, RPCRequestAttr(&v2pb.RpcRequest_ProcessList{ProcessList: &v2pb.ProcessListRequest{
		TopN: 50, SortBy: "mem",
	}}))
	req := out["request"].(map[string]any)
	if got := req["top_n"]; got.(float64) != 50 {
		t.Errorf("top_n = %v; want 50", got)
	}
	if got := req["sort_by"]; got != "mem" {
		t.Errorf("sort_by = %v; want mem", got)
	}
}

func TestRPCResponseAttr_ListDir(t *testing.T) {
	out := renderJSON(t, RPCResponseAttr(&v2pb.RpcResponse_ListDir{ListDir: &v2pb.ListDirResponse{
		Entries: []*v2pb.FileEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}},
	}}))
	resp := out["response"].(map[string]any)
	if got := resp["entry_count"]; got.(float64) != 3 {
		t.Errorf("entry_count = %v; want 3", got)
	}
}

func TestRPCResponseAttr_NilSafe(t *testing.T) {
	// nil / unknown payloads return an empty Group; slog elides those
	// from the rendered output, so the test only asserts no panic /
	// no spurious top-level keys leak. Error-path log lines therefore
	// omit `response` entirely rather than emitting `response: {}` —
	// this keeps the rendered shape free of dead fields.
	out := renderJSON(t, RPCResponseAttr(nil))
	if _, ok := out["response"]; ok {
		t.Fatalf("expected response group elided for nil payload; got %v", out)
	}
}

func must(b []byte, err error) string {
	if err != nil {
		panic(err)
	}
	return string(b)
}
