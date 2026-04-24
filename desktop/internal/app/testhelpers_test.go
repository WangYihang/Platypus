package app

import (
	"path/filepath"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

// freshConnectedApp builds a fresh App wired to the given base URL and
// returns it after a successful Connect. Shared by the dispatch,
// tunnels, and files test suites so every test that needs a
// connected app can stand one up in a single line.
func freshConnectedApp(t *testing.T, baseURL string) *App {
	t.Helper()
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-app-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", baseURL, "s")
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(a.Disconnect)
	return a
}
