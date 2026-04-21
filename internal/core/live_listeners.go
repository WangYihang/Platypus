package core

import (
	"errors"
)

// CoreLiveListeners adapts the package-level CreateTCPServer /
// DeleteServer pair to the api.LiveListeners interface the
// /projects/:pid/listeners handler depends on. Kept as a separate
// concrete type so the api package doesn't have to import core when it
// wants to test the handler — production wiring supplies an instance of
// this, tests supply a fake.
type CoreLiveListeners struct{}

// Create binds a new TLS-enabled TCPServer on host:port for the given
// project and returns its hash-based id. The hashFormat is inherited
// from the server default. Returns an error the handler surfaces as
// 502 Bad Gateway on bind failures.
func (CoreLiveListeners) Create(host string, port uint16, projectID string) (string, error) {
	s := CreateTCPServer(host, port, "", false, "", "")
	if s == nil {
		return "", errors.New("create tcp server failed; see server logs")
	}
	s.ProjectID = projectID
	go s.Run()
	return s.Hash, nil
}

// Delete stops the TCPServer matching id, if any. Never errors — a
// double-delete is harmless and the handler always drops the DB row
// regardless.
func (CoreLiveListeners) Delete(id string) error {
	srv, ok := Ctx.Servers[id].(*TCPServer)
	if !ok {
		return nil
	}
	srv.Stop()
	DeleteServer(srv)
	return nil
}
