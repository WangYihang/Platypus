package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

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

// AvailableRaasLanguages asks the server which one-liner languages it
// can render. Backed by GET /api/v1/raas/languages.
func (a *App) AvailableRaasLanguages() ([]string, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	body, err := c.Get(context.Background(), "/api/v1/raas/languages", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Languages []string `json:"languages"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse languages: %w", err)
	}
	sort.Strings(resp.Languages)
	return resp.Languages, nil
}

// GenerateRaasOneliner returns the shell command a victim should execute
// to call back to `listenerHostPort` (e.g. "1.2.3.4:13337"). Backed by
// GET /api/v1/raas/oneliner — the server is the single source of truth
// for every template, so changes land in one place.
func (a *App) GenerateRaasOneliner(listenerHostPort, lang string) (string, error) {
	c, err := a.client()
	if err != nil {
		return "", err
	}
	host, port := splitHostPort(listenerHostPort)
	q := url.Values{}
	q.Set("host", host)
	q.Set("port", port)
	q.Set("lang", lang)
	body, err := c.Get(context.Background(), "/api/v1/raas/oneliner", q)
	if err != nil {
		return "", err
	}
	var resp struct {
		Oneliner string `json:"oneliner"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse oneliner: %w", err)
	}
	return resp.Oneliner, nil
}

// splitHostPort splits "h:p" into host and port; returns sensible defaults
// if the input is malformed.
func splitHostPort(in string) (string, string) {
	if i := strings.LastIndex(in, ":"); i >= 0 {
		return in[:i], in[i+1:]
	}
	return in, "13337"
}
