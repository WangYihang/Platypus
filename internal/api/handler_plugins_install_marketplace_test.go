package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Coverage for the install_marketplace handler. Two paths:
//
//  1. Happy path: catalog has the row + the publisher key, fetcher
//     returns the artefacts, agent reports PHASE_INSTALLED.
//  2. Missing publisher key: catalog has the row but pubkey is empty
//     (legacy index without publisher_pubkey_b64) — the handler
//     short-circuits with 424 Failed Dependency, never opens the
//     mgmt stream.
//
// We don't re-cover the agent-side install pipeline here; that's
// owned by handler_plugins_v1_test.go's TestAgentPluginsV1_Install.
// This file's contract is "did the marketplace pre-stage step do
// the right thing?".

// stubFetcher is an in-memory ArtefactFetcher keyed by URL.
type stubFetcher map[string][]byte

func (s stubFetcher) Fetch(_ context.Context, url string) ([]byte, error) {
	if b, ok := s[url]; ok {
		return b, nil
	}
	return nil, http.ErrServerClosed // arbitrary non-nil error
}

func TestInstallFromMarketplace_HappyPath(t *testing.T) {
	wasm := []byte("FAKE-WASM-BYTES")
	manifest := []byte("api_version: 1\nid: com.example.x\nversion: 1.0.0\n")
	sig := []byte("MINISIG-FAKE")
	wasmHash := sha256.Sum256(wasm)

	catalogRow := MarketplaceRow{
		PluginID:        "com.example.x",
		Version:         "1.0.0",
		PublisherKeyID:  "abc123",
		PublisherPubkey: []byte("RWQfTRUSTED-KEY"),
		WasmURL:         "https://example.test/x.wasm",
		SignatureURL:    "https://example.test/x.wasm.minisig",
		ManifestURL:     "https://example.test/plugin.yaml",
		WasmSHA256Hex:   hex.EncodeToString(wasmHash[:]),
	}

	a := setupPluginsAgent(t, "agent-mkt-happy",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			install := req.GetInstall()
			if install == nil {
				t.Errorf("expected install op")
				return
			}
			if string(install.GetPublisherPubkey()) != string(catalogRow.PublisherPubkey) {
				t.Errorf("publisher pubkey not threaded through: got %q",
					install.GetPublisherPubkey())
			}
			if install.GetPluginId() != "com.example.x" {
				t.Errorf("plugin_id = %q", install.GetPluginId())
			}
			// Drain the three install chunks the handler pushes.
			for i := 0; i < 3; i++ {
				var c v2pb.PluginInstallChunk
				_ = link.ReadFrame(stream, &c)
			}
			// Emit a terminal PHASE_INSTALLED so the handler renders
			// status="installed".
			_ = link.WriteFrame(stream, &v2pb.PluginInstallProgress{
				Phase: v2pb.PluginInstallProgress_PHASE_INSTALLED,
			})
		},
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentPluginsHandler(a.svc).WithMarketplace(
		CatalogFunc(func(_ context.Context, id, ver string) (MarketplaceRow, bool, error) {
			if id == catalogRow.PluginID && ver == catalogRow.Version {
				return catalogRow, true, nil
			}
			return MarketplaceRow{}, false, nil
		}),
		stubFetcher{
			catalogRow.ManifestURL:  manifest,
			catalogRow.WasmURL:      wasm,
			catalogRow.SignatureURL: sig,
		},
	)
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.example.x","version":"1.0.0","granted_capabilities":["log"]}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins/install_marketplace", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}

	var got installResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "installed" {
		t.Errorf("status = %q, want installed", got.Status)
	}
}

func TestInstallFromMarketplace_NoPublisherKeyReturns424(t *testing.T) {
	a := setupPluginsAgent(t, "agent-mkt-nokey",
		func(_ *v2pb.PluginMgmtRequest, _ io.ReadWriteCloser) {
			t.Errorf("agent should not receive any mgmt op when pubkey is missing")
		},
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentPluginsHandler(a.svc).WithMarketplace(
		CatalogFunc(func(_ context.Context, id, ver string) (MarketplaceRow, bool, error) {
			return MarketplaceRow{
				PluginID:     id,
				Version:      ver,
				WasmURL:      "https://example/wasm",
				SignatureURL: "https://example/sig",
				ManifestURL:  "https://example/manifest",
				// PublisherPubkey deliberately empty.
			}, true, nil
		}),
		stubFetcher{
			"https://example/wasm":     []byte("ignored"),
			"https://example/sig":      []byte("ignored"),
			"https://example/manifest": []byte("ignored"),
		},
	)
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.example.x","version":"1.0.0"}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins/install_marketplace", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFailedDependency {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 424; body = %s", resp.StatusCode, b)
	}
}

func TestInstallFromMarketplace_NoCatalogReturns503(t *testing.T) {
	a := setupPluginsAgent(t, "agent-mkt-nocat",
		func(_ *v2pb.PluginMgmtRequest, _ io.ReadWriteCloser) {
			t.Errorf("agent should not receive any mgmt op when catalog is unwired")
		},
	)
	// No WithMarketplace decoration → catalog + fetcher are nil.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentPluginsHandler(a.svc)
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.example.x","version":"1.0.0"}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins/install_marketplace", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 503; body = %s", resp.StatusCode, b)
	}
}
