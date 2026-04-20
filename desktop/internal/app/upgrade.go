package app

import (
	"context"
	"net/url"
)

// UpgradeToTermite asks the server to compile + push a Termite agent over
// an existing plain reverse-shell session, replacing the channel with an
// encrypted one. Progress is broadcast as notify:upgrade:* events on the
// /notify WebSocket; the frontend consumes those to drive a progress bar.
//
// Returns once the request is acknowledged (HTTP 202) — the actual upgrade
// happens asynchronously on the server. Backed by
// POST /api/v1/sessions/{id}/upgrade with a JSON body {listener_id}.
func (a *App) UpgradeToTermite(plainSessionHash, targetListenerHash string) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	path := "/api/v1/sessions/" + url.PathEscape(plainSessionHash) + "/upgrade"
	_, err = c.Post(context.Background(), path, map[string]string{
		"listener_id": targetListenerHash,
	})
	return err
}
