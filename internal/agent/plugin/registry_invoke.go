package plugin

import (
	"context"
	"time"

	"github.com/WangYihang/Platypus/internal/agent"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Invoke is the single entry point bound to AgentRPCHandlers.PluginCall.
// It is safe for concurrent calls against different plugin ids; calls
// for the same plugin id serialise on loaded.mu (extism is not
// goroutine-safe).
func (r *Registry) Invoke(ctx context.Context, req *v2pb.PluginCallRequest) *v2pb.PluginCallResponse {
	start := time.Now()
	r.mu.RLock()
	l, ok := r.plugins[req.GetPluginId()]
	r.mu.RUnlock()

	resp := &v2pb.PluginCallResponse{}
	if !ok {
		resp.Error = "plugin_not_installed: " + req.GetPluginId()
		r.audit(req, resp, start, "")
		return resp
	}
	if !l.entry.Enabled {
		resp.Error = "plugin_disabled"
		r.audit(req, resp, start, "")
		return resp
	}
	if !exportDeclared(l.manifest, req.GetMethod()) {
		resp.Error = "method_not_declared: " + req.GetMethod()
		r.audit(req, resp, start, "")
		return resp
	}

	cctx := ctx
	if req.GetTimeoutMs() > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, time.Duration(req.GetTimeoutMs())*time.Millisecond)
		defer cancel()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	corr := agent.CorrelationIDFromContext(ctx)
	l.currentCorr.Store(&corr)

	inst, err := l.instanceOf(cctx)
	if err != nil {
		resp.Error = "instantiate: " + err.Error()
		r.audit(req, resp, start, corr)
		return resp
	}
	_, out, err := inst.CallWithContext(cctx, req.GetMethod(), req.GetPayload())
	if err != nil {
		resp.Error = "call: " + err.Error()
		r.audit(req, resp, start, corr)
		return resp
	}
	resp.Payload = out
	r.audit(req, resp, start, corr)
	return resp
}

// audit composes the per-call AuditRecord and hands it to the
// configured Auditor. Centralised so every Invoke success / failure
// path emits the same shape.
func (r *Registry) audit(req *v2pb.PluginCallRequest, resp *v2pb.PluginCallResponse, start time.Time, corr string) {
	r.mu.RLock()
	l, ok := r.plugins[req.GetPluginId()]
	r.mu.RUnlock()
	rec := AuditRecord{
		PluginID:      req.GetPluginId(),
		Method:        req.GetMethod(),
		CorrelationID: corr,
		ElapsedMS:     time.Since(start).Milliseconds(),
		Error:         resp.GetError(),
		FuelUsed:      resp.GetFuelUsed(),
		MemPeakBytes:  resp.GetMemPeakBytes(),
	}
	if ok {
		rec.GrantedCapabilities = l.entry.GrantedCapabilities
	}
	r.auditor(rec)
}

// exportDeclared returns true when manifest.rpc[].name == method.
// Plugins MUST list every export they intend to be called from the
// server, even if the wasm binary technically exports more — this is
// part of the install-time disclosure ("here are the methods this
// plugin will respond to").
func exportDeclared(m *Manifest, method string) bool {
	for _, r := range m.RPC {
		if r.Name == method {
			return true
		}
	}
	return false
}
