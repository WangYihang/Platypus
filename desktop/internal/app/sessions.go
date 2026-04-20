package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/WangYihang/Platypus/desktop/internal/api"
)

// listSessionsV1Response is the shape of GET /api/v1/sessions.
type listSessionsV1Response struct {
	Sessions []json.RawMessage `json:"sessions"`
}

// ListSessions returns every session (TCPClient + TermiteClient) attached
// to any listener. Backed by GET /api/v1/sessions.
func (a *App) ListSessions() ([]api.Session, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	body, err := c.Get(context.Background(), "/api/v1/sessions", nil)
	if err != nil {
		return nil, err
	}
	var resp listSessionsV1Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse /api/v1/sessions: %w", err)
	}

	out := make([]api.Session, 0, len(resp.Sessions))
	for _, raw := range resp.Sessions {
		var s api.Session
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		// TermiteClient has a "version" field; TCPClient does not.
		var probe map[string]any
		_ = json.Unmarshal(raw, &probe)
		if _, hasVersion := probe["version"]; hasVersion {
			s.Encrypted = true
			s.Tag = "termite"
		} else {
			s.Encrypted = false
			s.Tag = "shell"
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	return out, nil
}
