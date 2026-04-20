// Package keychain stores per-profile secrets in the OS-native credential
// vault (macOS Keychain, Windows Credential Vault, Linux secret-service)
// via github.com/zalando/go-keyring.
//
// This package handles only the secret material; profile metadata (name +
// URL) lives in the profile package.
package keychain

import (
	"errors"

	keyring "github.com/zalando/go-keyring"
)

// ErrNotFound is returned by Load when no secret exists for the given name.
var ErrNotFound = errors.New("keychain: secret not found")

// Store wraps go-keyring with a fixed service namespace.
type Store struct {
	service string
}

// New returns a Store scoped to the given service namespace. All Save/Load
// calls partition by this service so different apps (or test runs) don't
// collide.
func New(service string) *Store {
	return &Store{service: service}
}

// Save writes (or overwrites) the secret for name.
func (s *Store) Save(name, secret string) error {
	return keyring.Set(s.service, name, secret)
}

// Load retrieves the secret for name. Returns ErrNotFound if absent.
func (s *Store) Load(name string) (string, error) {
	v, err := keyring.Get(s.service, name)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return v, nil
}

// Delete removes the secret for name. It is idempotent: deleting a
// missing key returns nil (matching profile.Remove's contract is
// intentional — both surface "absent" as ErrNotFound only via Load/Get).
func (s *Store) Delete(name string) error {
	err := keyring.Delete(s.service, name)
	if err != nil && errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
