package api

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installRequest is the POST body for inline-source installs. The
// three artefacts are passed as base64 to keep JSON happy. URL-source
// installs (Phase 2) will accept {url:{wasm_url, signature_url, ...}}
// instead of the inline triple.
//
// granted_capabilities mirrors the operator-confirmed dialog: the
// agent enforces this set on every host-function call regardless of
// what the manifest claims, so under-grant is the safe default.
type installRequest struct {
	PluginID            string   `json:"plugin_id" binding:"required"`
	Version             string   `json:"version" binding:"required"`
	PublisherPubkey     string   `json:"publisher_pubkey" binding:"required"` // raw minisign .pub file contents
	ManifestB64         string   `json:"manifest_b64" binding:"required"`
	WasmB64             string   `json:"wasm_b64" binding:"required"`
	SignatureB64        string   `json:"signature_b64" binding:"required"`
	GrantedCapabilities []agentplugin.CapabilityID `json:"granted_capabilities"`
}

// installProgressJSON is the per-frame progress shape rendered in the
// JSON response. The REST endpoint blocks until a terminal phase, so
// the response carries the FULL progression rather than streaming it
// — small and bounded (~7 phases), and avoids forcing the UI to
// wire SSE for the MVP.
type installProgressJSON struct {
	Phase        string `json:"phase"`
	BytesDone    uint64 `json:"bytes_done,omitempty"`
	BytesTotal   uint64 `json:"bytes_total,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// installResponse is what the operator's UI sees.
type installResponse struct {
	Status   string                `json:"status"` // "installed" | "failed" | "in_progress"
	PluginID string                `json:"plugin_id"`
	Version  string                `json:"version"`
	Progress []installProgressJSON `json:"progress"`
}

// Install handles POST .../plugins. Mirrors the agent-upgrade endpoint
// shape but (a) carries the artefact bytes inline rather than fetching
// from a distributor, (b) drains PluginInstallProgress instead of
// UpgradeProgress, and (c) does not exit the agent on success.
func (h *AgentPluginsHandler) Install(c *gin.Context) {
	claims, _ := ClaimsFromContext(c)

	var body installRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manifestBytes, err := base64.StdEncoding.DecodeString(body.ManifestB64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manifest_b64: " + err.Error()})
		return
	}
	wasmBytes, err := base64.StdEncoding.DecodeString(body.WasmB64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wasm_b64: " + err.Error()})
		return
	}
	sigBytes, err := base64.StdEncoding.DecodeString(body.SignatureB64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "signature_b64: " + err.Error()})
		return
	}

	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_Install{Install: &v2pb.PluginInstallRequest{
			PluginId:        body.PluginID,
			Version:         body.Version,
			PublisherPubkey: []byte(body.PublisherPubkey),
			Source: &v2pb.PluginInstallRequest_Inline{Inline: &v2pb.PluginInlineSource{
				WasmSizeBytes: uint64(len(wasmBytes)),
			}},
			GrantedCapabilities: agentplugin.CapabilityIDsToStrings(body.GrantedCapabilities),
			Actor:               "user:" + claims.UserID,
		}},
	}
	stream, _, ok := h.openMgmtStream(c, req, "plugins-install")
	if !ok {
		return
	}
	defer func() { _ = stream.Close() }()

	// Push the three inline segments concurrently with the progress
	// drain so neither side deadlocks on a synchronous transport.
	go pushInstallChunks(stream, manifestBytes, wasmBytes, sigBytes)

	ctx, cancel := withDetachedTimeout(pluginInstallTimeout)
	defer cancel()

	progress, drainErr := drainInstallProgress(ctx, stream)
	resp := installResponse{
		PluginID: body.PluginID,
		Version:  body.Version,
		Progress: progress,
	}
	if len(progress) > 0 {
		last := progress[len(progress)-1]
		switch {
		case last.Phase == v2pb.PluginInstallProgress_PHASE_INSTALLED.String():
			resp.Status = "installed"
		case last.Phase == v2pb.PluginInstallProgress_PHASE_FAILED.String():
			resp.Status = "failed"
		default:
			resp.Status = "in_progress"
		}
	} else {
		resp.Status = "in_progress"
	}
	if drainErr != nil && resp.Status == "in_progress" {
		// Drain timed out / link dropped before a terminal phase.
		// Same status-code policy as the upgrade handler: 202 to flag
		// "we don't know how this finished, poll the audit log".
		c.JSON(http.StatusAccepted, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// pushInstallChunks frames the three install segments in the canonical
// order. Best-effort: drops write errors on the floor because if the
// agent has gone away mid-install, the drain side will surface that
// as an "in_progress" response.
func pushInstallChunks(w io.Writer, manifest, wasm, sig []byte) {
	for _, seg := range []struct {
		kind v2pb.PluginInstallChunk_Kind
		data []byte
	}{
		{v2pb.PluginInstallChunk_KIND_MANIFEST, manifest},
		{v2pb.PluginInstallChunk_KIND_WASM, wasm},
		{v2pb.PluginInstallChunk_KIND_SIGNATURE, sig},
	} {
		_ = link.WriteFrame(w, &v2pb.PluginInstallChunk{
			Kind: seg.kind, Data: seg.data, Last: true,
		})
	}
}

// drainInstallProgress reads PluginInstallProgress frames until the
// agent emits a terminal phase or the context expires. Returns the
// full progression (last entry is the terminal one on the happy path)
// plus any drain error (typically ctx.Err()).
func drainInstallProgress(ctx context.Context, stream io.ReadWriteCloser) ([]installProgressJSON, error) {
	type frameResult struct {
		p   v2pb.PluginInstallProgress
		err error
	}
	out := make([]installProgressJSON, 0, 8)
	for {
		ch := make(chan frameResult, 1)
		go func() {
			var p v2pb.PluginInstallProgress
			err := link.ReadFrame(stream, &p)
			ch <- frameResult{p, err}
		}()
		select {
		case fr := <-ch:
			if fr.err != nil {
				return out, fr.err
			}
			out = append(out, installProgressJSON{
				Phase:        fr.p.GetPhase().String(),
				BytesDone:    fr.p.GetBytesDone(),
				BytesTotal:   fr.p.GetBytesTotal(),
				ErrorCode:    fr.p.GetErrorCode(),
				ErrorMessage: fr.p.GetErrorMessage(),
			})
			switch fr.p.GetPhase() {
			case v2pb.PluginInstallProgress_PHASE_INSTALLED,
				v2pb.PluginInstallProgress_PHASE_FAILED:
				return out, nil
			}
		case <-ctx.Done():
			return out, ctx.Err()
		}
	}
}

