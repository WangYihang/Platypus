// Package cryptobox owns the project's at-rest sealing primitive: a
// single AES-256-GCM key (the KEK) loaded from the operator's
// environment, used to seal everything that the server stores
// encrypted — project CA private keys, project secrets, and any
// future at-rest sensitive blob.
//
// The package exists to break a would-be import cycle: both pki and
// storage seal blobs at rest, but pki already imports storage, so
// the seal/open helpers can't live in either of those packages
// without forming a loop. cryptobox depends on neither, so both
// callers can use it cleanly.
//
// The KEK itself is small: a 32-byte AES-256 key. There is one KEK
// per server installation (not per-project, despite the prefix on
// the env var). Per-project key derivation could come later via
// HKDF if multi-tenant isolation requirements demand it; today the
// shared-KEK posture matches the existing project_ca design.
package cryptobox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/WangYihang/Platypus/internal/log"
)

const (
	// EnvVar is where the AES-256 key-encryption-key lives.
	// Hex-encoded (64 chars). Empty or missing → operator hasn't
	// configured at-rest encryption; sealers refuse to operate
	// rather than silently downgrading.
	//
	// The env var name is shared with the legacy CA-only flow because
	// we want one KEK across the whole at-rest surface; operators
	// don't need to configure two.
	EnvVar = "PLATYPUS_CA_KEK"

	NonceLen = 12 // AES-GCM standard nonce length
	keyLen   = 32 // AES-256
)

// ErrKEKMissing is returned when EnvVar isn't set at the moment we
// need to seal / unseal a value. Handled as "encryption not
// configured" at the admin layer.
var ErrKEKMissing = errors.New("cryptobox: PLATYPUS_CA_KEK not set")

// ErrKEKMalformed is returned when the env var is set but isn't 32
// bytes of hex.
var ErrKEKMalformed = errors.New("cryptobox: PLATYPUS_CA_KEK must be 64 hex chars (32 bytes)")

// FilePath, when non-empty, enables a dev-friendly fallback: if
// EnvVar is unset, readKEK reads the hex-encoded KEK from this file,
// and if the file is missing it generates a random KEK and writes it
// there (0600). The server main sets this to "<data-dir>/ca.kek" so
// `docker compose up` works with zero config. Tests and strict
// "env var required" paths leave it empty.
//
// Trade-off when the fallback is active: the KEK sits next to the
// SQLite file it's supposed to protect, so encrypted blobs are
// effectively plaintext to anyone who can read the data volume. For
// production, set the env var explicitly (env takes precedence).
var FilePath string

var autoWarnOnce sync.Once

// Seal encrypts plaintext under the configured KEK with a fresh
// 12-byte nonce. Returns (nonce, ciphertext) separately so callers
// can persist them in distinct columns.
func Seal(plaintext []byte) (nonce, ciphertext []byte, err error) {
	kek, err := readKEK()
	if err != nil {
		return nil, nil, err
	}
	return aesGCMSeal(kek, plaintext)
}

// Open is the inverse of Seal. (nonce, ciphertext) → plaintext or
// error.
func Open(nonce, ciphertext []byte) ([]byte, error) {
	kek, err := readKEK()
	if err != nil {
		return nil, err
	}
	return aesGCMOpen(kek, nonce, ciphertext)
}

// readKEK loads the KEK in priority order:
//
//  1. EnvVar — production path; the operator wires the value in.
//  2. FilePath — dev fallback if the package var is set.
//  3. Generate a random KEK and persist it at FilePath, emitting a
//     one-shot WARN. Only reachable when FilePath is set.
//
// Returns ErrKEKMissing when neither env nor FilePath yields a
// value. ErrKEKMalformed when the source is present but not 32
// bytes of hex.
func readKEK() ([]byte, error) {
	if raw := os.Getenv(EnvVar); raw != "" {
		return decodeKEK(raw)
	}
	if FilePath == "" {
		return nil, ErrKEKMissing
	}

	data, err := os.ReadFile(FilePath)
	if err == nil {
		return decodeKEK(strings.TrimSpace(string(data)))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("cryptobox: read kek file %q: %w", FilePath, err)
	}

	kek := make([]byte, keyLen)
	if _, err := rand.Read(kek); err != nil {
		return nil, fmt.Errorf("cryptobox: generate kek: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(FilePath), 0o700); err != nil {
		return nil, fmt.Errorf("cryptobox: create kek dir: %w", err)
	}
	encoded := hex.EncodeToString(kek) + "\n"
	if err := os.WriteFile(FilePath, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("cryptobox: write kek file %q: %w", FilePath, err)
	}
	autoWarnOnce.Do(func() {
		log.L.Warn("auto_generated_ca_kek",
			"path", FilePath,
			"hint", "set PLATYPUS_CA_KEK in production to keep the key outside the data volume",
		)
	})
	return kek, nil
}

func decodeKEK(raw string) ([]byte, error) {
	kek, err := hex.DecodeString(raw)
	if err != nil || len(kek) != keyLen {
		return nil, ErrKEKMalformed
	}
	return kek, nil
}

func aesGCMSeal(kek, plaintext []byte) (nonce, ciphertext []byte, err error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, NonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return nonce, ciphertext, nil
}

func aesGCMOpen(kek, nonce, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}
