package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/desktop/internal/api"
)

// terminalSession couples an api.Terminal with its ID so we can route
// server-pushed frames to the right frontend tab.
type terminalSession struct {
	id   string
	term *api.Terminal
}

// terminalRegistry wraps the App's open terminal map. Its methods take the
// session lock; the App's own mu protects the registry pointer itself.
type terminalRegistry struct {
	mu    sync.Mutex
	byID  map[string]*terminalSession
}

func newTerminalRegistry() *terminalRegistry {
	return &terminalRegistry{byID: map[string]*terminalSession{}}
}

func (r *terminalRegistry) add(s *terminalSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[s.id] = s
}

func (r *terminalRegistry) get(id string) (*terminalSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	return s, ok
}

func (r *terminalRegistry) remove(id string) *terminalSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.byID[id]
	delete(r.byID, id)
	return s
}

func (r *terminalRegistry) drain() []*terminalSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*terminalSession, 0, len(r.byID))
	for id, s := range r.byID {
		out = append(out, s)
		delete(r.byID, id)
	}
	return out
}

// OpenTerminal opens a /ws/:sessionHash WebSocket and streams stdout
// to the frontend via terminal:output:<termID> events. Returns the new
// termID the frontend uses for all subsequent calls.
func (a *App) OpenTerminal(sessionHash string) (string, error) {
	a.mu.Lock()
	c := a.apiClient
	profile := a.connected
	terms := a.terminals
	if terms == nil {
		terms = newTerminalRegistry()
		a.terminals = terms
	}
	a.mu.Unlock()
	if c == nil {
		return "", ErrNotConnected
	}

	id := uuid.NewString()
	wsURL := terminalWSURL(profile.URL, sessionHash)

	h := &wailsTerminalHandler{id: id, app: a}
	term, err := api.DialTerminal(context.Background(), wsURL, h)
	if err != nil {
		return "", fmt.Errorf("dial terminal: %w", err)
	}
	terms.add(&terminalSession{id: id, term: term})
	return id, nil
}

// SendTerminalInput writes bytes to the remote PTY's stdin.
func (a *App) SendTerminalInput(termID string, data []byte) error {
	s, ok := a.lookupTerminal(termID)
	if !ok {
		return fmt.Errorf("terminal %q not found", termID)
	}
	return s.term.Write(data)
}

// ResizeTerminal notifies the remote PTY of new dimensions.
func (a *App) ResizeTerminal(termID string, cols, rows int) error {
	s, ok := a.lookupTerminal(termID)
	if !ok {
		return fmt.Errorf("terminal %q not found", termID)
	}
	return s.term.Resize(cols, rows)
}

// CloseTerminal closes the terminal and removes it from the registry.
func (a *App) CloseTerminal(termID string) error {
	a.mu.Lock()
	reg := a.terminals
	a.mu.Unlock()
	if reg == nil {
		return errors.New("terminal registry not initialised")
	}
	s := reg.remove(termID)
	if s == nil {
		return fmt.Errorf("terminal %q not found", termID)
	}
	s.term.Close()
	return nil
}

func (a *App) lookupTerminal(termID string) (*terminalSession, bool) {
	a.mu.Lock()
	reg := a.terminals
	a.mu.Unlock()
	if reg == nil {
		return nil, false
	}
	return reg.get(termID)
}

// closeAllTerminals tears down every open terminal. Called from Disconnect.
func (a *App) closeAllTerminals() {
	a.mu.Lock()
	reg := a.terminals
	a.terminals = nil
	a.mu.Unlock()
	if reg == nil {
		return
	}
	for _, s := range reg.drain() {
		s.term.Close()
	}
}

// terminalWSURL builds ws(s)://server/ws/<hash> from the saved http(s) URL.
func terminalWSURL(baseHTTPURL, sessionHash string) string {
	u := strings.TrimRight(baseHTTPURL, "/")
	switch {
	case strings.HasPrefix(u, "http://"):
		u = "ws://" + strings.TrimPrefix(u, "http://")
	case strings.HasPrefix(u, "https://"):
		u = "wss://" + strings.TrimPrefix(u, "https://")
	}
	return u + "/ws/" + sessionHash
}

// wailsTerminalHandler bridges api.TerminalHandler to App.emit.
type wailsTerminalHandler struct {
	id  string
	app *App
}

func (h *wailsTerminalHandler) OnOutput(data []byte) {
	// Base64-encoded so the WebView's JSON pipe doesn't mangle binary.
	h.app.emit("terminal:output:"+h.id, base64.StdEncoding.EncodeToString(data))
}
func (h *wailsTerminalHandler) OnTitle(title string) {
	h.app.emit("terminal:title:"+h.id, title)
}
func (h *wailsTerminalHandler) OnPreferences(prefs string) {
	h.app.emit("terminal:preferences:"+h.id, prefs)
}
func (h *wailsTerminalHandler) OnClose(err error) {
	payload := ""
	if err != nil {
		payload = err.Error()
	}
	h.app.emit("terminal:closed:"+h.id, payload)
}
