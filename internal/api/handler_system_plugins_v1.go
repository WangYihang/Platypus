package api

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// systemPluginsDirName is the basename inside the server's data dir
// that the dev publisher (scripts/dev-publish-entrypoint.sh) drops
// signed system-plugin bundles into when it wants to override the
// server binary's embedded set. The reconciler in
// handler_agent_link_v2.go pulls from this directory (or the embedded
// fallback when it's empty) on every agent connect.
const systemPluginsDirName = "system-plugins"

// SystemPluginsHandler exposes the catalogue of system-eligible
// plugins to the admin / enroll wizard. systemBundle is the fs.FS
// the resolver picked at boot (operator-staged disk override or the
// server binary's prebuilt tree); the handler can't tell which.
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
	systemBundle fs.FS
}

func NewSystemPluginsHandler(bundle fs.FS) *SystemPluginsHandler {
	return &SystemPluginsHandler{systemBundle: bundle}
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
	// OSTargets / ArchTargets mirror manifest.runtime.os_targets /
	// arch_targets. Empty slice = all platforms. Surfaced to the
	// wizard so the picker can grey-out plugins that don't apply
	// to the host being enrolled, and consumed by the reconciler
	// to skip incompatible plugins for an already-enrolled agent.
	OSTargets   []string `json:"os_targets,omitempty"`
	ArchTargets []string `json:"arch_targets,omitempty"`
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
	entries, err := enumerateSystemPlugins(h.systemBundle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list system plugins: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, systemPluginsResponse{Plugins: entries})
}

// enumerateSystemPlugins walks <plugin_id>/<version>/ for plugin.yaml
// entries on the supplied fs.FS and parses each. Returns a sorted
// slice (id asc, then version asc) so the wizard ordering is stable.
//
// nil fs.FS → returns ([], nil). The picker treats that as an empty
// state, not an error.
//
// Per-bundle parse failures are dropped silently; one corrupt
// manifest doesn't blank the whole picker.
func enumerateSystemPlugins(fsys fs.FS) ([]systemPluginInfo, error) {
	if fsys == nil {
		return []systemPluginInfo{}, nil
	}
	pluginDirs, err := fs.ReadDir(fsys, ".")
	if err != nil {
		// A truly absent root (e.g. an os.DirFS pointed at a missing
		// path) returns fs.ErrNotExist via the Open call inside
		// ReadDir. The picker treats that the same as "no bundles".
		if errors.Is(err, fs.ErrNotExist) {
			return []systemPluginInfo{}, nil
		}
		return nil, err
	}
	out := []systemPluginInfo{}
	for _, pd := range pluginDirs {
		if !pd.IsDir() {
			continue
		}
		versions, err := fs.ReadDir(fsys, pd.Name())
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			data, err := fs.ReadFile(fsys, path.Join(pd.Name(), v.Name(), "plugin.yaml"))
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
				OSTargets:    append([]string(nil), m.Runtime.OSTargets...),
				ArchTargets:  append([]string(nil), m.Runtime.ArchTargets...),
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
