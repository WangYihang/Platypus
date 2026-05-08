package link

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"google.golang.org/protobuf/proto"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Session is the v2 link session: a hashicorp/yamux multiplexer
// plus helpers that pair every new stream with a StreamHeader
// handshake. Both sides of the connection wrap the same underlying
// net.Conn in a Session; the yamux protocol is symmetric aside
// from the initial role negotiation.
//
// linkSessionID is a logging-only identifier. The server stamps it
// after Register so every StreamHeader emitted on this session can
// carry the same id without callers threading it through. Empty on
// the agent side until / unless we wire a Hello frame to push it
// down — empty stays a valid value (just renders as an empty field
// in the agent log line).
type Session struct {
	sess *yamux.Session

	mu            sync.RWMutex
	linkSessionID string
}

// NewClientSession wraps conn in the client role of yamux. The
// caller retains ownership of conn but Session.Close will close
// the yamux layer (which closes conn) — double-closing conn is
// safe on most net.Conn implementations.
func NewClientSession(conn net.Conn) (*Session, error) {
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard // silence yamux's periodic info logs
	sess, err := yamux.Client(conn, cfg)
	if err != nil {
		return nil, fmt.Errorf("link: yamux client: %w", err)
	}
	return &Session{sess: sess}, nil
}

// NewServerSession is the counterpart used on the server side of
// the connection.
func NewServerSession(conn net.Conn) (*Session, error) {
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	sess, err := yamux.Server(conn, cfg)
	if err != nil {
		return nil, fmt.Errorf("link: yamux server: %w", err)
	}
	return &Session{sess: sess}, nil
}

// SetLinkSessionID stamps the per-link logging id on this session.
// Called by the server's AgentLinkService once the agent registers;
// the value is then echoed automatically in every StreamHeader
// produced by Open. Idempotent — safe to call again on a reconnect
// path that reuses the Session pointer.
func (s *Session) SetLinkSessionID(id string) {
	s.mu.Lock()
	s.linkSessionID = id
	s.mu.Unlock()
}

// LinkSessionID returns the per-link logging id. Empty when not yet
// set (e.g. the agent side, or before Register on the server side).
func (s *Session) LinkSessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.linkSessionID
}

// Close tears down the yamux session (and the underlying conn).
// Pending streams get io.EOF on subsequent reads.
func (s *Session) Close() error {
	return s.sess.Close()
}

// Open initiates a new yamux stream and writes a StreamHeader
// carrying the supplied type, correlation id, and pre-marshalled
// service-specific metadata (often a ProcessOpenRequest, RpcRequest,
// etc. — the caller marshals with marshalMeta).
// The per-link logging id is taken off the Session itself so every
// stream over the same session inherits the same value without the
// caller threading it through.
//
// The returned ReadWriteCloser is the live stream; the caller is
// responsible for reading the peer's accept/reject reply (one
// WriteFrame of StreamResponse, not yet wired) and then issuing
// service-specific I/O.
func (s *Session) Open(t v2pb.StreamType, metadata []byte, correlationID string) (io.ReadWriteCloser, error) {
	stream, err := s.sess.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("link: open stream: %w", err)
	}
	hdr := &v2pb.StreamHeader{
		Type:          t,
		Metadata:      metadata,
		CorrelationId: correlationID,
		LinkSessionId: s.LinkSessionID(),
	}
	if err := WriteFrame(stream, hdr); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("link: write header: %w", err)
	}
	return stream, nil
}

// Accept blocks for the next incoming stream, reads and parses its
// StreamHeader, and returns both to the caller. Caller decides
// whether to handle or reject based on StreamHeader.Type and then
// continues reading per-stream-type frames. An accepted stream is
// just a raw io.ReadWriteCloser; any further framing is the
// handler's responsibility.
//
// Returns io.EOF when the peer has closed the session cleanly.
func (s *Session) Accept() (*v2pb.StreamHeader, io.ReadWriteCloser, error) {
	stream, err := s.sess.AcceptStream()
	if err != nil {
		if errors.Is(err, yamux.ErrSessionShutdown) {
			return nil, nil, io.EOF
		}
		return nil, nil, fmt.Errorf("link: accept stream: %w", err)
	}
	var hdr v2pb.StreamHeader
	if err := ReadFrame(stream, &hdr); err != nil {
		_ = stream.Close()
		return nil, nil, fmt.Errorf("link: read header: %w", err)
	}
	// Mirror the link-session id back onto the Session the first time
	// we see one. Lets the agent side learn its server-assigned id
	// from the very first stream the server opens — typically the
	// initial sysinfo refresh, fired right after link.connected
	// — so subsequent agent-initiated streams (events) carry it too
	// without a dedicated Hello frame.
	if id := hdr.GetLinkSessionId(); id != "" && s.LinkSessionID() == "" {
		s.SetLinkSessionID(id)
	}
	return &hdr, stream, nil
}

// marshalMeta is a tiny convenience wrapper so callers don't have
// to import google.golang.org/protobuf just to produce the bytes
// for StreamHeader.Metadata. Returns nil + nil for a nil message.
func marshalMeta(m proto.Message) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return proto.Marshal(m)
}

// unmarshalMeta is the inverse. Treats an empty metadata slice as
// a no-op (the header had no service-specific payload attached).
func unmarshalMeta(b []byte, m proto.Message) error {
	if len(b) == 0 {
		return nil
	}
	return proto.Unmarshal(b, m)
}
