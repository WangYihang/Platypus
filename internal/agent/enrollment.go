package agent

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/update"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// credentialPrefixes identifies strings that look like PAT / session
// tokens. Anything else is treated as a legacy bearer token (passed
// through without enrollment), preserving compatibility with older
// server builds.
const (
	patPrefix     = "plt_"
	sessionPrefix = "sess_"
)

// EnrollmentResult is what the server returned. On success the caller
// must persist SessionToken to disk before the next reconnect.
type EnrollmentResult struct {
	Attempted        bool
	Succeeded        bool
	AgentID          string
	SessionToken     string
	SessionExpiresAt time.Time
	ErrorMessage     string

	// Mesh bootstrap fields from the server.
	MeshPSK       []byte
	MeshPeers     []string
	MeshProjectID string
}

// RenewalContext is handed to StartRenewalLoop so it can rotate the
// session token mid-connection. It's a shrink-wrapped view of the
// agent's live Client and identity dir — keeps the package boundaries
// clean without forcing callers to pass the whole agent state.
type RenewalContext struct {
	Client      *Client
	IdentityDir string
	// CurrentToken is the session_token plaintext that MaybeEnroll just
	// persisted; the renewal loop uses this as the first "current"
	// value and then tracks rotations itself.
	CurrentToken string
	// ExpiresAt is when the server says the current token dies. The
	// loop fires the renewal at `ExpiresAt - renewalMarginBeforeExpiry`.
	ExpiresAt time.Time
}

// renewalMarginBeforeExpiry is how early we attempt rotation. The
// server sends a recommended_renew_at in AgentEnrollResponse but when
// the agent didn't get one (e.g. enrollment was skipped and we're
// resuming with a session file) we fall back to this.
const renewalMarginBeforeExpiry = 6 * time.Hour

// StartRenewalLoop schedules an in-band session rotation before the
// current session_token expires. Returns immediately; the rotation
// runs in a goroutine. Passing a zero ExpiresAt disables the loop
// (common on legacy paths where no session was negotiated).
//
// The loop is intentionally single-shot per-call: once it rotates
// successfully, it schedules itself again from the new expiry. If the
// TLS connection drops mid-loop the timer fires harmlessly against a
// closed codec and returns.
func StartRenewalLoop(ctx RenewalContext, stop <-chan struct{}) {
	if ctx.Client == nil || ctx.ExpiresAt.IsZero() || ctx.CurrentToken == "" {
		return
	}
	go runRenewalLoop(ctx, stop)
}

func runRenewalLoop(rctx RenewalContext, stop <-chan struct{}) {
	current := rctx.CurrentToken
	expiresAt := rctx.ExpiresAt
	for {
		wait := time.Until(expiresAt.Add(-renewalMarginBeforeExpiry))
		// A min floor keeps a misbehaving server (or clock skew) from
		// hot-looping us; a max ceiling is unnecessary because we
		// trust expires_at.
		if wait < 30*time.Second {
			wait = 30 * time.Second
		}
		select {
		case <-stop:
			return
		case <-time.After(wait):
		}

		next, newExpires, err := requestRenewal(rctx.Client, current)
		if err != nil {
			log.Warn("Session renew RPC failed: %s", err)
			return
		}
		if next == "" {
			// Server rejected. The RPC logged the reason; there's
			// nothing productive the loop can do — Connect's main
			// read path will notice the connection soon.
			return
		}
		if err := persistSessionToken(rctx.IdentityDir, next); err != nil {
			log.Warn("Failed to persist rotated session token: %s", err)
			// Keep going anyway; the in-memory `current` is still
			// valid for this connection.
		}
		log.Success("Session token rotated (new expiry %s)", newExpires.Format("2006-01-02 15:04:05"))
		current = next
		expiresAt = newExpires
	}
}

// requestRenewal sends a SessionRenewRequest on the live connection and
// waits for the paired SessionRenewResponse. It matches on a fresh
// request_id so replies can't collide with unrelated RPC responses.
//
// Returns (plaintext, newExpiresAt, nil) on success, ("", zero, nil)
// on a server-side rejection (error string non-empty), and ("", zero, err)
// on transport failures.
func requestRenewal(c *Client, current string) (string, time.Time, error) {
	reqID := fmt.Sprintf("renew-%d", time.Now().UnixNano())
	env := &agentpb.Envelope{
		Version:   protocolVersion,
		Timestamp: time.Now().UnixNano(),
		RequestId: reqID,
		Payload: &agentpb.Envelope_SessionRenewRequest{
			SessionRenewRequest: &agentpb.SessionRenewRequest{
				CurrentSessionToken: current,
			},
		},
	}
	// Handshake over the normal codec. We don't install a receiver
	// goroutine — the response comes back in the same frame we're
	// reading from, but on the agent side nothing else is reading
	// from the connection concurrently because HandleConnection is
	// the sole consumer and it filters by message type.
	//
	// Wait, that last statement is wrong — HandleConnection IS running
	// and will grab this frame first. We route responses back through
	// it via a channel registered before sending; see renewalResponseCh.
	ch, cancel := registerRenewalWaiter(reqID)
	defer cancel()

	if err := c.SendEnvelope(env); err != nil {
		return "", time.Time{}, fmt.Errorf("send renew: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != "" {
			log.Warn("Server rejected session renew: %s", resp.Error)
			return "", time.Time{}, nil
		}
		return resp.SessionToken, time.Unix(resp.SessionExpiresAt, 0), nil
	case <-time.After(30 * time.Second):
		return "", time.Time{}, fmt.Errorf("renew response timeout")
	}
}

// renewalWaiters is the small per-process registry that bridges
// HandleConnection (which owns reads) and the renewal loop (which sends
// a request and needs to read the matching response). When an envelope
// of type SessionRenewResponse arrives, HandleConnection looks up the
// request_id and forwards the inner response on the channel.
var renewalWaiters = struct {
	sync.Mutex
	byID map[string]chan *agentpb.SessionRenewResponse
}{byID: map[string]chan *agentpb.SessionRenewResponse{}}

// registerRenewalWaiter allocates a one-shot channel keyed by request_id.
// The returned cancel func removes the entry on defer regardless of
// whether the response arrived — prevents the map from growing when
// the server misbehaves or the timeout fires.
func registerRenewalWaiter(id string) (<-chan *agentpb.SessionRenewResponse, func()) {
	ch := make(chan *agentpb.SessionRenewResponse, 1)
	renewalWaiters.Lock()
	renewalWaiters.byID[id] = ch
	renewalWaiters.Unlock()
	return ch, func() {
		renewalWaiters.Lock()
		delete(renewalWaiters.byID, id)
		renewalWaiters.Unlock()
	}
}

// deliverRenewalResponse is called by the message-handling loop when a
// SessionRenewResponse comes in. Silent no-op if nobody's waiting
// (shouldn't happen, but we'd rather log-and-drop than deadlock).
func deliverRenewalResponse(reqID string, resp *agentpb.SessionRenewResponse) {
	renewalWaiters.Lock()
	ch, ok := renewalWaiters.byID[reqID]
	renewalWaiters.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- resp:
	default:
	}
}

// MaybeEnroll optionally runs the agent-side enrollment handshake. It
// picks a credential from (in order) a persisted session file or the
// `--token` CLI argument. If neither is a valid PAT / session token the
// function returns {Attempted: false} and the caller stays on the
// legacy path. Returning Attempted=true with Succeeded=false means the
// server rejected us and the connection must be abandoned — see
// internal/agent/agent.go:Connect for the caller.
func MaybeEnroll(c *Client, token, identityDir string) (*EnrollmentResult, error) {
	raw, source, err := chooseCredential(token, identityDir)
	if err != nil {
		return &EnrollmentResult{}, err
	}
	if raw == "" {
		return &EnrollmentResult{Attempted: false}, nil
	}

	// Build an enrollment request. Filling machine_id and hostname
	// lets the server correlate / enforce binding without a
	// round-trip.
	mid := MachineID()
	host, _ := os.Hostname()
	uname := "unknown"
	if u, err := user.Current(); err == nil && u != nil {
		uname = u.Username
	}
	_ = uname // reserved for future pubkey signing; silence unused warning

	req := &agentpb.AgentEnrollRequest{
		Credential: raw,
		MachineId:  mid,
		Hostname:   host,
		Os:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Version:    update.Version,
	}
	if err := c.SendEnvelope(&agentpb.Envelope{
		Version: protocolVersion, Timestamp: time.Now().UnixNano(),
		Payload: &agentpb.Envelope_AgentEnrollRequest{AgentEnrollRequest: req},
	}); err != nil {
		return &EnrollmentResult{}, fmt.Errorf("agent: send enroll: %w", err)
	}

	// Server must reply with exactly one frame. Bound the wait so a
	// misbehaving / wrong-version server can't pin us forever.
	env, err := recvWithDeadline(c, 5*time.Second)
	if err != nil {
		return &EnrollmentResult{}, fmt.Errorf("agent: recv enroll response: %w", err)
	}
	resp, ok := env.Payload.(*agentpb.Envelope_AgentEnrollResponse)
	if !ok {
		return &EnrollmentResult{}, fmt.Errorf("agent: unexpected first reply %T", env.Payload)
	}
	r := resp.AgentEnrollResponse
	if r.Error != "" {
		log.Error("Enrollment rejected by server: %s", r.Error)
		return &EnrollmentResult{
			Attempted: true, Succeeded: false,
			ErrorMessage: r.Error,
		}, nil
	}

	// Persist the freshly-issued session token ASAP. If we crash after
	// this point the next restart picks up where we left off without
	// needing the PAT again.
	if err := persistSessionToken(identityDir, r.SessionToken); err != nil {
		log.Warn("Failed to persist session token (%s): %s", identityDir, err)
		// Non-fatal — enrollment still happened on the server side,
		// but the next restart will need another PAT.
	}
	_ = source // source is informative for the log line only

	return &EnrollmentResult{
		Attempted:        true,
		Succeeded:        true,
		AgentID:          r.AgentId,
		SessionToken:     r.SessionToken,
		SessionExpiresAt: time.Unix(r.SessionExpiresAt, 0),
		MeshPSK:          r.MeshPsk,
		MeshPeers:        r.MeshPeers,
		MeshProjectID:    r.MeshProjectId,
	}, nil
}

// chooseCredential picks which credential to present, in priority order:
//  1. A previously persisted session_token file
//  2. The --token CLI argument if it looks like a PAT
//
// Anything else → empty (signal: stay legacy).
func chooseCredential(token, identityDir string) (cred, source string, err error) {
	if identityDir == "" {
		identityDir = defaultIdentityDir()
	}
	// 1) Session file
	path := sessionTokenPath(identityDir)
	if data, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(data))
		if strings.HasPrefix(s, sessionPrefix) {
			return s, "session-file", nil
		}
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("read session file: %w", err)
	}
	// 2) --token
	if strings.HasPrefix(token, patPrefix) || strings.HasPrefix(token, sessionPrefix) {
		return token, "cli-token", nil
	}
	return "", "", nil
}

// persistSessionToken writes the session_token to <identityDir>/session.token
// with mode 0600. Creates the directory with 0700 if it doesn't exist.
func persistSessionToken(identityDir, token string) error {
	if identityDir == "" {
		identityDir = defaultIdentityDir()
	}
	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		return err
	}
	tmp := sessionTokenPath(identityDir) + ".tmp"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	// Atomic rename so a crash never leaves a truncated file.
	return os.Rename(tmp, sessionTokenPath(identityDir))
}

func sessionTokenPath(identityDir string) string {
	return filepath.Join(identityDir, "session.token")
}

func defaultIdentityDir() string {
	// If no directory is specified, we generate a random temporary one.
	// This ensures that multiple agents can run on the same host without
	// ever colliding on session.token or mesh keys, achieving true
	// "zero-config" isolation.
	dir, err := os.MkdirTemp("", "platypus-agent-*")
	if err != nil {
		// Fallback to a hidden directory in the current working directory
		// if temp dir creation fails.
		return ".platypus-agent-fallback"
	}
	return dir
}

// recvWithDeadline applies a read deadline to the underlying TLS conn
// for a single Recv call. Clears the deadline before returning so the
// normal read path in HandleConnection isn't affected.
func recvWithDeadline(c *Client, d time.Duration) (*agentpb.Envelope, error) {
	type deadlineSetter interface {
		SetReadDeadline(time.Time) error
	}
	ds, ok := any(c.Conn).(deadlineSetter)
	if !ok || c.Conn == nil {
		return c.RecvEnvelope()
	}
	if err := ds.SetReadDeadline(time.Now().Add(d)); err != nil {
		return nil, err
	}
	defer func() { _ = ds.SetReadDeadline(time.Time{}) }()

	env, err := c.RecvEnvelope()
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, fmt.Errorf("timeout waiting for enroll response")
		}
		return nil, err
	}
	return env, nil
}
