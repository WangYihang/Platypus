package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// systemPluginsDirName is the basename inside the server's data dir
// that the dev publisher (scripts/dev-publish-entrypoint.sh) drops
// signed system-plugin bundles into. Matches the agent-side override
// dir name in internal/agent/plugin/system/embed.go (OverrideSubdir).
const systemPluginsDirName = "system-plugins"

// SystemPluginsHandler exposes the catalogue of system-eligible
// plugins to the admin / enroll wizard. Source of truth is
// <DataDir>/system-plugins/, the same directory the publisher
// stages and the agent's data-dir override resolver picks from.
//
// Two reasons to keep this distinct from the marketplace handler:
//
//  1. System plugins are signed by the SYSTEM publisher key, not
//     the marketplace publisher key. Mixing them would force the
//     wizard to render two different "trust this signer?"
//     confirmations.
//  2. System plugins are pre-installable from the install bundle
//     (Phase A's baseline_plugin_ids). Marketplace plugins are
//     ALWAYS post-enroll opt-in. The semantics differ enough that
//     conflating the two would lead to surprising operator decisions.
type SystemPluginsHandler struct {
	dataDir string
}

func NewSystemPluginsHandler(dataDir string) *SystemPluginsHandler {
	return &SystemPluginsHandler{dataDir: dataDir}
}

// systemPluginInfo is the wire shape — small enough to be obvious
// from the manifest at a glance, with the operator-visible fields
// the wizard / picker needs.
type systemPluginInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	Author       string   `json:"author,omitempty"`
	License      string   `json:"license,omitempty"`
	Capabilities []string `json:"capabilities"`
	// Streams reflects which v2pb.StreamType values this plugin
	// claims. Useful for the picker to surface "this plugin owns
	// file_read" without needing the wizard to know the manifest
	// shape.
	Streams []string `json:"streams,omitempty"`
}

type systemPluginsResponse struct {
	Plugins []systemPluginInfo `json:"plugins"`
}

// List returns the directory of system plugins discoverable on this
// server. Empty list (with HTTP 200) when the publisher hasn't
// staged anything yet — the FE renders an empty-state hint pointing
// the operator at the docker-compose dev flow / production seeding
// docs. We intentionally don't 503 on "no system plugins available"
// since the request itself is well-formed.
func (h *SystemPluginsHandler) List(c *gin.Context) {
	root := filepath.Join(h.dataDir, systemPluginsDirName)
	entries, err := enumerateSystemPlugins(root)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list system plugins: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, systemPluginsResponse{Plugins: entries})
}

// enumerateSystemPlugins walks root/<plugin_id>/<version>/ for
// plugin.yaml entries and parses each. Returns a sorted slice (id
// asc, then version asc) so the wizard ordering is stable.
//
// Missing root → returns ([], nil). The picker treats that as an
// empty state, not an error.
//
// Per-bundle parse failures are logged on stderr (best-effort) and
// excluded from the result; one corrupt manifest doesn't blank the
// whole picker.
func enumerateSystemPlugins(root string) ([]systemPluginInfo, error) {
	st, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []systemPluginInfo{}, nil
		}
		return nil, err
	}
	if !st.IsDir() {
		// Someone put a file at /system-plugins. Treat as no bundles.
		return []systemPluginInfo{}, nil
	}

	out := []systemPluginInfo{}
	pluginDirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, pd := range pluginDirs {
		if !pd.IsDir() {
			continue
		}
		pluginRoot := filepath.Join(root, pd.Name())
		versions, err := os.ReadDir(pluginRoot)
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			manifestPath := filepath.Join(pluginRoot, v.Name(), "plugin.yaml")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
			m, err := plugin.ParseManifest(data)
			if err != nil {
				continue
			}
			caps := m.DeclaredCapabilities()
			capStrs := make([]string, 0, len(caps))
			for _, c := range caps {
				capStrs = append(capStrs, string(c))
			}
			streams := make([]string, 0, len(m.Streams))
			for _, s := range m.Streams {
				if s.StreamType != "" {
					streams = append(streams, s.StreamType)
				}
			}
			out = append(out, systemPluginInfo{
				ID:           m.ID,
				Name:         m.Name,
				Version:      m.Version,
				Description:  m.Description,
				Author:       m.Author.Name,
				License:      m.License,
				Capabilities: capStrs,
				Streams:      streams,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// RegisterV1SystemPluginsRoutes mounts the system-plugins surface.
// Authenticated reads only — the picker surfaces in the enroll
// wizard which already requires admin auth, but we don't restrict
// to a project role since the catalog is server-wide.
func RegisterV1SystemPluginsRoutes(engine *gin.Engine, h *SystemPluginsHandler, rbac *RBAC) {
	engine.GET("/api/v1/system-plugins",
		rbac.RequireAuth(),
		h.List,
	)
}
