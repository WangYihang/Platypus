// Package optoken provides the shared opaque-token primitives used by
// every credential the platypus server hands out: PATs (plt_), AI agent
// tokens (aat_), persistent user sessions (pst_), install download
// tokens (dl_), and any future kind. One generator, one parser, one
// hash, one constant-time compare — so every credential's wire format
// and storage shape stay identical and the audit story is uniform.
//
// Token shape: "<prefix><id_b32>.<secret_b32>". The id half is the
// primary key in storage; the secret half is base32-decoded and
// compared (constant time) against a stored sha256 digest.
//
// This package deliberately knows nothing about HTTP, storage, or RBAC.
// It's the lowest layer of the auth stack — kinds and their semantics
// live in callers (Verifier, RBAC).
package optoken

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
)

const (
	idLen     = 20
	secretLen = 20
	// 13 raw bytes encode to 21 base32 chars; we slice to 20 to keep the
	// id/secret human-stable and free of the trailing padding-equivalent
	// character produced by the 13→21 expansion.
	rawLen = 13
)

// enc is unpadded uppercase base32 (RFC 4648). We lowercase on emit for
// log-grep stability; Parse uppercases on decode, so case round-trips.
var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// ErrMalformed signals any input that fails the prefix / dot / base32
// shape check. Callers map this to a 401 with a generic message — the
// exact subreason stays internal so we don't help token-format probes.
var ErrMalformed = errors.New("optoken: malformed token")

// Generate produces a fresh (id, secret, hash, plaintext) tuple for a
// caller-supplied prefix. The plaintext is the only string the user
// ever sees; secret/hash are what storage holds. The caller is
// responsible for persisting (id, hash) and returning plaintext exactly
// once.
func Generate(prefix string) (id string, secret, hash []byte, plaintext string, err error) {
	idRaw := make([]byte, rawLen)
	secretRaw := make([]byte, rawLen)
	if _, err = rand.Read(idRaw); err != nil {
		return "", nil, nil, "", fmt.Errorf("optoken: read id entropy: %w", err)
	}
	if _, err = rand.Read(secretRaw); err != nil {
		return "", nil, nil, "", fmt.Errorf("optoken: read secret entropy: %w", err)
	}
	idPart := strings.ToLower(enc.EncodeToString(idRaw))[:idLen]
	secretPart := strings.ToLower(enc.EncodeToString(secretRaw))[:secretLen]
	id = prefix + idPart

	// The hash is over the *decoded* secret bytes so it round-trips with
	// what Parse returns. Re-decoding our own emit is the simplest way
	// to guarantee that contract.
	secret, err = enc.DecodeString(strings.ToUpper(secretPart))
	if err != nil {
		return "", nil, nil, "", fmt.Errorf("optoken: decode fresh secret: %w", err)
	}
	hash = Hash(secret)
	plaintext = id + "." + secretPart
	return id, secret, hash, plaintext, nil
}

// Parse splits a presented token into (id, secret) using the supplied
// expected prefix. Returns ErrMalformed for anything that doesn't match
// `<expectedPrefix><id_b32>.<secret_b32>` or whose secret half isn't
// valid base32. Parse never touches storage — verification is the
// caller's job.
func Parse(raw, expectedPrefix string) (id string, secret []byte, err error) {
	if expectedPrefix == "" || !strings.HasPrefix(raw, expectedPrefix) {
		return "", nil, ErrMalformed
	}
	rest := strings.TrimPrefix(raw, expectedPrefix)
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", nil, ErrMalformed
	}
	secret, decErr := enc.DecodeString(strings.ToUpper(parts[1]))
	if decErr != nil {
		return "", nil, ErrMalformed
	}
	return expectedPrefix + parts[0], secret, nil
}

// Hash returns sha256(secret). Wrapped so storage code never imports
// crypto/sha256 directly and so a future migration (e.g. blake3) lands
// in one place.
func Hash(secret []byte) []byte {
	sum := sha256.Sum256(secret)
	return sum[:]
}

// Equal compares two byte slices in constant time. Returns true iff the
// slices have identical length and contents. Safe for hash comparison
// — never short-circuits on length, never branches on content.
func Equal(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
