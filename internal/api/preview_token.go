package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

// PreviewSigner mints + verifies short-lived URL tokens for browser
// elements (<video>, <audio>) that cannot carry an Authorization
// header. The signing key is generated fresh on process start and held
// only in memory, so URLs do not survive a restart — that's a feature,
// not a bug: the 5-minute TTL is short enough that a fresh mint is
// cheap, and not persisting the secret means a database breach can't
// be replayed into long-lived previews.
//
// Wire shape, modeled on AWS S3 presigned URLs:
//
//	GET /api/v1/projects/<pid>/agents/<aid>/fs/read
//	    ?path=<urlencoded path>
//	    &exp=<unix-seconds>
//	    &preview_token=<base64url HMAC-SHA256>
//
// The token signs the canonical string
//
//	"<pid>|<agentID>|<path>|<exp>"
//
// Every URL field that bound the issuer's intent must therefore match
// the verifier's view exactly — flipping any one of them invalidates
// the signature, so a token minted for /etc/passwd cannot be reused to
// read /etc/shadow on the same agent.
type PreviewSigner struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// DefaultPreviewTTL is how long a freshly-minted preview URL stays
// valid. Long enough to survive a slow seek to the end of a video;
// short enough that an accidentally-leaked URL can't be replayed for
// hours. 5 minutes is the same horizon S3 / Cloud Storage default to
// for one-shot presigned URLs.
const DefaultPreviewTTL = 5 * time.Minute

// NewPreviewSigner reads 32 bytes of OS entropy as the HMAC secret.
// Process-local; persistence is intentionally not offered.
func NewPreviewSigner() (*PreviewSigner, error) {
	s := make([]byte, 32)
	if _, err := rand.Read(s); err != nil {
		return nil, fmt.Errorf("preview signer: read entropy: %w", err)
	}
	return &PreviewSigner{secret: s, ttl: DefaultPreviewTTL, now: time.Now}, nil
}

// Sign returns (token, exp). exp is the unix-seconds wall-clock the
// caller embeds into the URL; both fields ride next to each other so
// the verifier can distinguish a token tampered to extend its TTL
// (signature mismatch) from a legitimately-expired one (signature ok,
// now > exp).
func (s *PreviewSigner) Sign(projectID, agentID, path string) (string, int64) {
	exp := s.now().Add(s.ttl).Unix()
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(canonicalPreviewString(projectID, agentID, path, exp)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), exp
}

// Verify rebuilds the canonical string from the URL's view of the
// world and checks the supplied token matches. Returns false on any
// of: malformed token, signature mismatch, or expired exp.
func (s *PreviewSigner) Verify(projectID, agentID, path string, exp int64, token string) bool {
	if token == "" {
		return false
	}
	given, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(canonicalPreviewString(projectID, agentID, path, exp)))
	if !hmac.Equal(given, mac.Sum(nil)) {
		return false
	}
	if s.now().Unix() > exp {
		return false
	}
	return true
}

// canonicalPreviewString centralises the field-ordering / delimiter
// choice so Sign and Verify can never disagree. The pipe is safe
// because none of the fields can legitimately contain one (project
// ids are UUIDs, agent ids are lowercase hex, exp is a number; path
// can technically contain pipes but the entire string is HMAC'd as
// raw bytes so collisions only matter cross-field, which the leading
// length-stable fields prevent in practice).
func canonicalPreviewString(projectID, agentID, path string, exp int64) string {
	return projectID + "|" + agentID + "|" + path + "|" + strconv.FormatInt(exp, 10)
}
