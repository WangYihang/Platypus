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
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".platypus", "agent")
	}
	return filepath.Join(home, ".platypus", "agent")
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
