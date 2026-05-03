package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installMarketplaceRequest is the POST body for the
// "fetch from marketplace + install on agent" path. The server
// looks up the catalog row by (plugin_id, version), fetches the
// three URLs server-side (no CORS pain on the operator's browser),
// verifies the wasm sha256, and feeds the bytes into the same
// agent install stream the inline-source endpoint uses.
//
// granted_capabilities mirrors the operator-confirmed dialog: the
// agent enforces this set on every host_fn call.
type installMarketplaceRequest struct {
	PluginID            string   `json:"plugin_id" binding:"required"`
	Version             string   `json:"version" binding:"required"`
	GrantedCapabilities []string `json:"granted_capabilities"`
}

// InstallFromMarketplace handles
// POST /api/v1/projects/:pid/agents/:agent_id/plugins/install_marketplace.
// Path that exists alongside the inline-source POST .../plugins,
// reusing the same agent-side install stream.
func (h *AgentPluginsHandler) InstallFromMarketplace(c *gin.Context) {
	if h.catalog == nil || h.fetcher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "marketplace catalog not configured on this server",
		})
		return
	}
	claims, _ := ClaimsFromContext(c)

	var body installMarketplaceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Catalog lookup with a tight timeout. The DB read is local SQLite
	// — anything slower than ~1s here is an indexing pathology, surface
	// loudly rather than waiting forever.
	lookupCtx, lookupCancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer lookupCancel()
	row, ok, err := h.catalog.Get(lookupCtx, body.PluginID, body.Version)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "catalog lookup: " + err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf(
			"no marketplace entry for %s@%s", body.PluginID, body.Version)})
		return
	}
	if row.WasmURL == "" || row.SignatureURL == "" || row.ManifestURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf(
			"catalog row for %s@%s is missing artefact URLs", body.PluginID, body.Version)})
		return
	}

	// Fetch the three artefacts concurrently. Total budget bounded by
	// pluginInstallTimeout — the agent install path also caps the
	// stream lifetime.
	fetchCtx, fetchCancel := context.WithTimeout(c.Request.Context(), pluginInstallTimeout)
	defer fetchCancel()

	manifestBytes, wasmBytes, sigBytes, fetchErr := fetchArtefacts(fetchCtx, h.fetcher, row)
	if fetchErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fetchErr.Error()})
		return
	}

	// Defence-in-depth: agent re-verifies sha256 + minisign, but
	// failing here gives the operator a clearer error than
	// "verify_sig failed mid-stream".
	if row.WasmSHA256Hex != "" {
		got := sha256.Sum256(wasmBytes)
		if hex.EncodeToString(got[:]) != row.WasmSHA256Hex {
			c.JSON(http.StatusBadRequest, gin.H{"error": "wasm sha256 mismatch — catalog out of sync, refresh the marketplace"})
			return
		}
	}

	if len(row.PublisherPubkey) == 0 {
		// Catalog has the plugin row but no signing key. Without it
		// the agent can't verify the wasm and would refuse the
		// install anyway — short-circuit with a clear actionable
		// error: the upstream index needs a publisher_pubkey_b64
		// alongside this version.
		c.JSON(http.StatusFailedDependency, gin.H{"error": fmt.Sprintf(
			"catalog has no publisher key for %s@%s — re-sync against an index that publishes publisher_pubkey_b64",
			body.PluginID, body.Version)})
		return
	}
	publisherPubkey := row.PublisherPubkey
	_ = fetchCtx // reserved if the publisher key ever needs a separate fetch

	// Stream into the same agent endpoint the inline-source path
	// uses. The agent doesn't know or care that the bytes came from
	// the marketplace — same verify_sig + sha + load pipeline.
	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_Install{Install: &v2pb.PluginInstallRequest{
			PluginId:        body.PluginID,
			Version:         body.Version,
			PublisherPubkey: publisherPubkey,
			Source: &v2pb.PluginInstallRequest_Inline{Inline: &v2pb.PluginInlineSource{
				WasmSizeBytes: uint64(len(wasmBytes)),
			}},
			GrantedCapabilities: body.GrantedCapabilities,
			Actor:               "user:" + claims.UserID,
		}},
	}
	stream, _, opened := h.openMgmtStream(c, req, "plugins-install-marketplace")
	if !opened {
		return
	}
	defer func() { _ = stream.Close() }()

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
		c.JSON(http.StatusAccepted, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// fetchArtefacts grabs the three URLs in parallel. Returns
// (manifest, wasm, sig, err) — the first error wins; the other
// goroutines are abandoned (their results discarded by the
// timeout / WaitGroup return).
func fetchArtefacts(ctx context.Context, f ArtefactFetcher, row MarketplaceRow) ([]byte, []byte, []byte, error) {
	type result struct {
		idx  int
		body []byte
		err  error
	}
	urls := [3]string{row.ManifestURL, row.WasmURL, row.SignatureURL}
	labels := [3]string{"manifest", "wasm", "signature"}
	results := make(chan result, 3)
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			b, err := f.Fetch(ctx, url)
			if err != nil {
				err = fmt.Errorf("fetch %s (%s): %w", labels[i], url, err)
			}
			results <- result{idx: i, body: b, err: err}
		}(i, url)
	}
	wg.Wait()
	close(results)

	var out [3][]byte
	for r := range results {
		if r.err != nil {
			return nil, nil, nil, r.err
		}
		out[r.idx] = r.body
	}
	return out[0], out[1], out[2], nil
}

// (Publisher key now flows from the catalog row — see the row
// lookup above which carries .PublisherPubkey populated by the
// catalog refresh from the index's publisher_pubkey_b64 field.)

// CatalogFunc adapts any catalog-shape Get function into the
// MarketplaceCatalog interface this package needs. Lets main.go
// wire `*coreplugin.Catalog.Get` without having to import
// internal/core/plugin from this file (which would force the
// dependency direction back the other way).
type CatalogFunc func(ctx context.Context, pluginID, version string) (MarketplaceRow, bool, error)

// Get implements MarketplaceCatalog.
func (f CatalogFunc) Get(ctx context.Context, pluginID, version string) (MarketplaceRow, bool, error) {
	return f(ctx, pluginID, version)
}

// httpArtefactFetcher is the production net/http-backed implementation
// of ArtefactFetcher. Bounded body read to keep an attacker-controlled
// URL from streaming gigabytes into RAM; the agent's wasm size limit
// is the actual ceiling but a server-side first-line of defence is
// cheap.
type httpArtefactFetcher struct {
	client *http.Client
	maxBytes int64
}

// NewHTTPArtefactFetcher wires a default-tuned http.Client + a 64 MiB
// per-fetch cap. A wasm plugin bigger than 64 MiB on disk is well
// outside the design envelope; the cap can be re-tuned later if
// real plugins push it.
func NewHTTPArtefactFetcher() ArtefactFetcher {
	return &httpArtefactFetcher{
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
		maxBytes: 64 << 20,
	}
}

func (f *httpArtefactFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
}
