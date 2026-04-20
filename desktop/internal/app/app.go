// Package app exposes the Wails-bindable App struct. Methods on App are
// auto-bound and called from the React frontend; this file is the single
// surface for that contract.
package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/desktop/internal/api"
	"github.com/WangYihang/Platypus/desktop/internal/keychain"
	"github.com/WangYihang/Platypus/desktop/internal/profile"
)

// ConnectionStatus is returned to the frontend so it can render the
// "Connected to <name>" state.
type ConnectionStatus struct {
	Connected   bool   `json:"connected"`
	ProfileName string `json:"profileName"`
	URL         string `json:"url"`
}

// App is the Wails-bound application struct.
type App struct {
	ctx       context.Context
	registry  *profile.Registry
	keychain  *keychain.Store
	apiClient *api.Client
	connected profile.Profile
	mu        sync.Mutex
}

// New constructs an App. configPath is where profile metadata is persisted;
// keychainService scopes secret storage in the OS vault. Production wiring
// in main.go fills these from os.UserConfigDir(); tests inject t.TempDir().
func New(configPath, keychainService string) (*App, error) {
	reg, err := profile.NewRegistry(configPath)
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	return &App{
		registry: reg,
		keychain: keychain.New(keychainService),
	}, nil
}

// Startup is the wails OnStartup hook; we cache ctx so future EventsEmit
// calls (D5+) can fire from background goroutines.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// ListProfiles returns every saved server profile (without secrets).
func (a *App) ListProfiles() ([]profile.Profile, error) {
	return a.registry.List(), nil
}

// AddProfile saves a new server connection. Secret goes to the OS keychain;
// (name, url) lives in the profile registry on disk.
func (a *App) AddProfile(name, url, secret string) error {
	p := profile.Profile{Name: name, URL: url}
	if err := a.registry.Add(p); err != nil {
		return err
	}
	if err := a.keychain.Save(name, secret); err != nil {
		// Roll back the registry add so we don't leave dangling metadata.
		_ = a.registry.Remove(name)
		return fmt.Errorf("save secret: %w", err)
	}
	return a.registry.Save()
}

// RemoveProfile drops both the metadata and the secret.
func (a *App) RemoveProfile(name string) error {
	if err := a.registry.Remove(name); err != nil {
		return err
	}
	if err := a.keychain.Delete(name); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	a.mu.Lock()
	if a.apiClient != nil && a.connected.Name == name {
		a.apiClient = nil
		a.connected = profile.Profile{}
	}
	a.mu.Unlock()
	return a.registry.Save()
}

// Connect resolves the named profile, exchanges the secret for a token,
// and parks the resulting api.Client for subsequent calls.
func (a *App) Connect(name string) error {
	p, ok := a.registry.Get(name)
	if !ok {
		return profile.ErrNotFound
	}
	secret, err := a.keychain.Load(name)
	if err != nil {
		return fmt.Errorf("load secret: %w", err)
	}

	client := api.NewClient(p.URL, "")
	ctx, cancel := context.WithTimeout(connectCtx(a), 15*time.Second)
	defer cancel()
	if err := client.FetchToken(ctx, secret); err != nil {
		return err
	}

	a.mu.Lock()
	a.apiClient = client
	a.connected = p
	a.mu.Unlock()
	return nil
}

// Disconnect drops the active session. Profile metadata + secret stay.
func (a *App) Disconnect() {
	a.mu.Lock()
	a.apiClient = nil
	a.connected = profile.Profile{}
	a.mu.Unlock()
}

// ConnectionStatus reflects whether Connect has been called successfully.
func (a *App) ConnectionStatus() ConnectionStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return ConnectionStatus{
		Connected:   a.apiClient != nil,
		ProfileName: a.connected.Name,
		URL:         a.connected.URL,
	}
}

// client returns the active api.Client. Callers (D5+ methods) check
// ErrNotConnected before issuing requests.
//
//nolint:unused // wired up in D5
func (a *App) client() (*api.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.apiClient == nil {
		return nil, ErrNotConnected
	}
	return a.apiClient, nil
}

// ErrNotConnected is returned when methods are called before Connect.
var ErrNotConnected = errors.New("app: not connected; call Connect first")

// connectCtx returns a.ctx if Wails has called Startup, otherwise
// context.Background. Tests run without Startup so they need the fallback.
func connectCtx(a *App) context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
