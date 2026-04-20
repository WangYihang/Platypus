package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/WangYihang/Platypus/desktop/internal/api"
)

// SetGroupDispatch toggles the server-side group_dispatch flag on one
// session. Sessions with the flag set receive commands from DispatchCommand.
// Backed by PATCH /api/v1/sessions/:id.
func (a *App) SetGroupDispatch(sessionHash string, enabled bool) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	body := map[string]any{"group_dispatch": enabled}
	_, err = c.Patch(context.Background(), "/api/v1/sessions/"+url.PathEscape(sessionHash), body)
	return err
}

// DispatchCommand runs command on every session with group_dispatch=true
// and collects the output. timeoutSec ≤0 falls back to the server default (3s).
// Per-session timeouts surface in the result's Error field rather than as a
// request-level error.
func (a *App) DispatchCommand(command string, timeoutSec int) ([]api.DispatchResult, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	body := map[string]any{"command": command, "timeout": timeoutSec}
	resp, err := c.Post(context.Background(), "/api/v1/sessions/dispatch", body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []api.DispatchResult `json:"results"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, fmt.Errorf("parse dispatch response: %w", err)
	}
	return parsed.Results, nil
}
