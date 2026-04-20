package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/WangYihang/Platypus/desktop/internal/api"
)

// ListTunnels returns active tunnels for the given session.
// Backed by GET /api/v1/sessions/:id/tunnels.
func (a *App) ListTunnels(sessionID string) ([]api.TunnelInfo, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	body, err := c.Get(context.Background(),
		"/api/v1/sessions/"+url.PathEscape(sessionID)+"/tunnels", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status  bool             `json:"status"`
		Tunnels []api.TunnelInfo `json:"tunnels"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse tunnels: %w", err)
	}
	return resp.Tunnels, nil
}

// CreateTunnel sets up a new tunnel on the session. mode is one of
// "pull" | "push" | "dynamic" | "internet"; the address fields are
// interpreted per mode (see internal/api/handler_tunnel.go on the
// server for the exact semantics).
func (a *App) CreateTunnel(sessionID, mode, srcAddress, dstAddress string) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	body := map[string]string{
		"mode":        mode,
		"src_address": srcAddress,
		"dst_address": dstAddress,
	}
	_, err = c.Post(context.Background(),
		"/api/v1/sessions/"+url.PathEscape(sessionID)+"/tunnels", body)
	return err
}
