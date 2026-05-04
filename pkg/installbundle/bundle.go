// Package installbundle is the wire codec for the self-contained
// install token format `pinst_<base64>`.
//
// A bundle carries the three pieces an agent needs to bootstrap on a
// machine that can't reach the server during install:
//
//	server      — host:port the agent should dial after enrollment
//	pat         — single-use enrollment token (`plt_<id>.<secret>`)
//	ca_pem      — project CA the agent pins for mTLS
//	project_id  — informational; the agent also derives this from
//	              the issued cert's URI SAN, but having it in the
//	              bundle lets the agent log it before enrollment
//	              completes
//
// The bundle is base64(JSON) prefixed with `pinst_`. Choosing JSON
// over a binary protobuf keeps the codec auditable in 80 lines and
// makes the token survive accidental log-line truncation (the prefix
// is unmistakable, so an operator pasting a partial token gets a
// clear "looks truncated" parse error rather than a silent
// short-cert).
package installbundle

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Prefix is the canonical magic string. Anything starting with this
// is treated as a bundle by the agent CLI; anything else is passed
// through as a raw PAT (the legacy code path).
const Prefix = "pinst_"

// Bundle is the decoded shape. JSON tags pin the wire field names so
// a future Go-struct rename can't accidentally break older agents
// reading newer tokens.
type Bundle struct {
	// Schema is the bundle format version. Forward compat: agents
	// that don't recognise a version refuse to parse rather than
	// silently decoding the unknown shape (which might be missing
	// a future security-relevant field).
	Schema int `json:"v"`

	// Server is the host:port to dial. Stored as the literal string
	// the admin entered in the install-mint dialog (typically the
	// server's public_addr).
	Server string `json:"server"`

	// PAT is the plaintext enrollment token (`plt_<id>.<secret>`).
	// Single-use; consumed by /api/v1/agents/enroll on first call.
	PAT string `json:"pat"`

	// CACertPEM is the project CA's certificate PEM. The agent pins
	// this for mTLS to the server endpoint above. Empty when the
	// project hasn't initialised a CA — agents fall back to the
	// `PLATYPUS_INSECURE_DOWNLOAD=1` opt-in.
	CACertPEM string `json:"ca_pem,omitempty"`

	// ProjectID is the project this bundle enrols into. Carried for
	// agent-side logging before the cert is in hand; the
	// authoritative source is still the issued cert's URI SAN.
	ProjectID string `json:"project_id,omitempty"`
}

// CurrentSchema is the value Encode stamps on every bundle. Bumped
// when an incompatible field shape lands; old agents seeing a higher
// number must error out (see Decode).
const CurrentSchema = 1

// minSupportedSchema is the oldest schema version Decode accepts.
const minSupportedSchema = 1

// ErrMalformed is returned by Decode for any parse failure (missing
// prefix, bad base64, bad JSON, unknown schema). Callers map this to
// a "looks like an install bundle but couldn't parse" error message.
var ErrMalformed = errors.New("installbundle: malformed bundle")

// Encode serialises b into the wire format. b.Schema is overridden
// with CurrentSchema; Server and PAT are required (empty strings
// produce an error so we never silently emit a token that won't
// boot an agent).
func Encode(b Bundle) (string, error) {
	if b.Server == "" {
		return "", errors.New("installbundle: Encode: Server required")
	}
	if b.PAT == "" {
		return "", errors.New("installbundle: Encode: PAT required")
	}
	b.Schema = CurrentSchema
	raw, err := json.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("installbundle: marshal: %w", err)
	}
	// URL-safe base64 with no padding so the token slots into URLs,
	// shell heredocs, and copy-paste flows without quoting concerns.
	return Prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// Decode parses a wire bundle. Returns ErrMalformed for any failure
// mode the caller wants to render as "this isn't a valid install
// bundle"; non-malformed errors (future schema, etc.) get a typed
// error so the agent log line can distinguish them.
func Decode(raw string) (*Bundle, error) {
	if !strings.HasPrefix(raw, Prefix) {
		return nil, ErrMalformed
	}
	body := strings.TrimPrefix(raw, Prefix)
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, fmt.Errorf("%w: base64: %v", ErrMalformed, err)
	}
	var b Bundle
	if err := json.Unmarshal(decoded, &b); err != nil {
		return nil, fmt.Errorf("%w: json: %v", ErrMalformed, err)
	}
	if b.Schema < minSupportedSchema || b.Schema > CurrentSchema {
		// Out of supported window: refuse rather than silently dropping
		// fields the older code doesn't know about, or accepting a
		// future shape we can't safely interpret.
		return nil, fmt.Errorf("installbundle: unsupported schema v=%d (this build understands v=%d..v=%d)",
			b.Schema, minSupportedSchema, CurrentSchema)
	}
	if b.Server == "" || b.PAT == "" {
		return nil, fmt.Errorf("%w: missing required field (server / pat)", ErrMalformed)
	}
	return &b, nil
}

// Looks reports whether raw looks like a bundle (right prefix). A
// quick check the agent CLI uses to choose the parsing path; the
// actual round-trip happens in Decode.
func Looks(raw string) bool {
	return strings.HasPrefix(raw, Prefix)
}
