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
// Returns once the request is acknowledged — the actual upgrade happens
// asynchronously on the server.
func (a *App) UpgradeToTermite(plainSessionHash, targetListenerHash string) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	path := "/api/client/" + url.PathEscape(plainSessionHash) +
		"/upgrade/" + url.PathEscape(targetListenerHash)
	_, err = c.Get(context.Background(), path, nil)
	return err
}
