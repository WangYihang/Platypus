package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// List is the agent-side view consumed by PluginListResponse.
func (r *Registry) List() []*v2pb.PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*v2pb.PluginInfo, 0, len(r.plugins))
	for _, l := range r.plugins {
		out = append(out, &v2pb.PluginInfo{
			Id:             l.id,
			Name:           l.manifest.Name,
			Version:        l.entry.Version,
			Author:         l.manifest.Author.Name,
			Enabled:        l.entry.Enabled,
			Capabilities:   l.entry.GrantedCapabilities,
			InstallUnix:    uint64(l.entry.InstalledAt.Unix()),
			SourceUrl:      l.entry.SourceURL,
			PublisherKeyId: l.entry.PublisherKeyID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetId() < out[j].GetId() })
	return out
}

// Tail returns the most recent N log lines for one plugin, or
// (nil, os.ErrNotExist) when the plugin id is unknown.
func (r *Registry) Tail(pluginID string, n int) ([]*v2pb.PluginLogEntry, error) {
	r.mu.RLock()
	l, ok := r.plugins[pluginID]
	r.mu.RUnlock()
	if !ok {
		return nil, os.ErrNotExist
	}
	return l.logs.Tail(n), nil
}

// SetEnabled flips the enabled flag in the catalog and updates the
// in-memory entry. Disabled plugins stay loaded (cheap) but Invoke
// returns error="plugin disabled" without entering the wasm runtime.
func (r *Registry) SetEnabled(pluginID string, enabled bool) error {
	if err := r.catalog.SetEnabled(pluginID, enabled); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.plugins[pluginID]; ok {
		l.entry.Enabled = enabled
	}
	return nil
}

// Remove uninstalls one plugin: drops it from the catalog, closes the
// instance, deletes the on-disk version directory. When purgeState is
// true the plugin's state/ dir is also removed; otherwise it's
// preserved for a future reinstall.
//
// Note on state preservation: the current layout has state/ inside
// PluginDir, so an uninstall removes it either way. The purge_state
// flag is wired through for forward compatibility with the planned
// Phase 2 split that will move state/ outside PluginDir so it can
// genuinely survive a remove+install cycle.
func (r *Registry) Remove(ctx context.Context, pluginID string, purgeState bool) error {
	_ = purgeState // see comment above
	r.mu.Lock()
	l, ok := r.plugins[pluginID]
	if ok {
		l.close(ctx)
		delete(r.plugins, pluginID)
	}
	r.mu.Unlock()

	if err := r.catalog.Remove(pluginID); err != nil {
		return fmt.Errorf("plugin: catalog remove: %w", err)
	}
	if err := os.RemoveAll(r.paths.PluginDir(pluginID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("plugin: remove install dir: %w", err)
	}
	return nil
}
