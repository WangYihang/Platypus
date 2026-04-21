package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"

	"github.com/WangYihang/Platypus/desktop/internal/api"
)

// listListenersV1Response is the shape of GET /api/v1/listeners.
type listListenersV1Response struct {
	Listeners []struct {
		api.Listener
		Clients        map[string]any `json:"clients"`
		TermiteClients map[string]any `json:"termite_clients"`
	} `json:"listeners"`
}

// ListListeners returns every TCPServer registered on the server, with a
// computed NumSessions = #plain + #termite clients.
func (a *App) ListListeners() ([]api.Listener, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	body, err := c.Get(context.Background(), "/api/v1/listeners", nil)
	if err != nil {
		return nil, err
	}
	var resp listListenersV1Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse /api/v1/listeners: %w", err)
	}
	out := make([]api.Listener, 0, len(resp.Listeners))
	for _, s := range resp.Listeners {
		l := s.Listener
		l.NumSessions = len(s.Clients) + len(s.TermiteClients)
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	return out, nil
}

// CreateListener spawns a new reverse-shell listener on the server via the
// JSON-native POST /api/v1/listeners.
func (a *App) CreateListener(host string, port int, encrypted bool) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	_, err = c.Post(context.Background(), "/api/v1/listeners", map[string]any{
		"host":      host,
		"port":      port,
		"encrypted": encrypted,
	})
	return err
}

// DeleteListener tears down a listener by hash.
func (a *App) DeleteListener(hash string) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	_, err = c.Delete(context.Background(), "/api/v1/listeners/"+url.PathEscape(hash))
	return err
}
