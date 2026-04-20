// Package profile manages the user's saved server connection profiles.
//
// A Profile is just (Name, URL); secrets live in the keychain package.
// The Registry persists profiles as a JSON file, typically at
// ~/.config/platypus-desktop/profiles.json.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var (
	ErrNotFound      = errors.New("profile: not found")
	ErrAlreadyExists = errors.New("profile: already exists")
)

// Profile is the on-disk representation of a saved server connection.
type Profile struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Registry holds the in-memory set of profiles backed by a JSON file.
// Methods are safe for concurrent use.
type Registry struct {
	path     string
	profiles map[string]Profile
	mu       sync.Mutex
}

// NewRegistry loads (or initialises) a Registry at the given file path.
// A missing file is treated as an empty registry — Save creates it lazily.
func NewRegistry(path string) (*Registry, error) {
	r := &Registry{path: path, profiles: map[string]Profile{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return r, nil
	}
	var stored []Profile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, p := range stored {
		r.profiles[p.Name] = p
	}
	return r, nil
}

func validate(p Profile) error {
	if p.Name == "" {
		return errors.New("profile: name is required")
	}
	if p.URL == "" {
		return errors.New("profile: url is required")
	}
	u, err := url.Parse(p.URL)
	if err != nil {
		return fmt.Errorf("profile: invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("profile: url scheme must be http or https, got %q", u.Scheme)
	}
	return nil
}

// Add inserts a new profile. Returns ErrAlreadyExists if Name is taken.
func (r *Registry) Add(p Profile) error {
	if err := validate(p); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[p.Name]; exists {
		return ErrAlreadyExists
	}
	r.profiles[p.Name] = p
	return nil
}

// Update replaces an existing profile. Returns ErrNotFound if missing.
func (r *Registry) Update(p Profile) error {
	if err := validate(p); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[p.Name]; !exists {
		return ErrNotFound
	}
	r.profiles[p.Name] = p
	return nil
}

// Get returns the profile by name. The bool reports presence.
func (r *Registry) Get(name string) (Profile, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.profiles[name]
	return p, ok
}

// Remove deletes a profile. Returns ErrNotFound if absent.
func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[name]; !exists {
		return ErrNotFound
	}
	delete(r.profiles, name)
	return nil
}

// List returns all profiles sorted by name.
func (r *Registry) List() []Profile {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Profile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Save persists the registry to disk, creating parent dirs as needed.
// Writes to a temp file and renames for atomic-ish updates.
func (r *Registry) Save() error {
	r.mu.Lock()
	snapshot := make([]Profile, 0, len(r.profiles))
	for _, p := range r.profiles {
		snapshot = append(snapshot, p)
	}
	r.mu.Unlock()
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].Name < snapshot[j].Name })

	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, r.path, err)
	}
	return nil
}
