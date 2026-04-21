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
		TermiteClients map[string]any `json:"termite_clients"`
	} `json:"listeners"`
}

// ListListeners returns every TCPServer registered on the server, with a
// computed NumSessions = #agents dialled in.
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
		l.NumSessions = len(s.TermiteClients)
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	return out, nil
}

// CreateListener opens a new TLS ingress on the server.
func (a *App) CreateListener(host string, port int) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	_, err = c.Post(context.Background(), "/api/v1/listeners", map[string]any{
		"host": host,
		"port": port,
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
