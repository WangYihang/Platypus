package link

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
)

// Subprotocol is the WebSocket Sec-WebSocket-Protocol header value
// both sides negotiate. Bumping this string means "incompatible v2
// wire format" — readers on the current code path refuse to speak
// to peers that didn't offer it, so old/new agents don't silently
// talk past each other.
const Subprotocol = "ptps-link-v2"

// DialOptions configures a single Dial attempt. TLSConfig is only
// consulted for wss:// URLs (e.g. built by BuildDialerTLSConfig on
// the agent side). HTTPClient is an optional override for tests;
// production callers leave it nil and get a fresh client with
// TLSConfig applied to the transport.
type DialOptions struct {
	URL        string
	TLSConfig  *tls.Config
	HTTPClient *http.Client
}

// ErrPendingApproval signals the server returned 425 Too Early on the
// WS upgrade — i.e. the host is enrolled but waiting for an admin to
// click Approve. Agent retry loops should keep going on a long
// backoff and surface a friendly status message instead of dumping
// the raw error.
var ErrPendingApproval = errors.New("link: agent awaiting admin approval")

// ErrApprovalRejected signals the server returned 403 with the
// "rejected" approval header — admin explicitly denied this host.
// Cert is dead from the server's perspective; the agent should exit
// rather than retry forever.
var ErrApprovalRejected = errors.New("link: agent enrollment rejected by administrator")

// Dial performs the agent-side link bring-up: WebSocket Upgrade to
// opts.URL (with the v2 subprotocol), then wrap the resulting
// connection in a yamux client Session. On any step's failure the
// WS connection is closed before returning.
//
// Dial classifies two well-known server-side rejections so the agent
// retry loop can decide between "wait quietly" and "exit loudly":
//   - 425 Too Early              → ErrPendingApproval
//   - 403 + X-Platypus-Approval-Status: rejected → ErrApprovalRejected
func Dial(ctx context.Context, opts DialOptions) (*Session, error) {
	if opts.URL == "" {
		return nil, errors.New("link: Dial: URL required")
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: opts.TLSConfig,
			},
		}
	}

	c, resp, err := websocket.Dial(ctx, opts.URL, &websocket.DialOptions{
		HTTPClient:   httpClient,
		Subprotocols: []string{Subprotocol},
	})
	if err != nil {
		// Server can attach an X-Platypus-Approval-Status header on
		// the failed upgrade response. coder/websocket exposes the
		// response on dial failure (non-nil) so we can read it
		// without re-dialling. Match the status header first; fall
		// back to plain HTTP code matching for older servers.
		if resp != nil {
			switch resp.Header.Get("X-Platypus-Approval-Status") {
			case "pending":
				return nil, ErrPendingApproval
			case "rejected":
				return nil, ErrApprovalRejected
			}
			if resp.StatusCode == http.StatusTooEarly {
				return nil, ErrPendingApproval
			}
			if resp.StatusCode == http.StatusForbidden {
				return nil, ErrApprovalRejected
			}
		}
		return nil, fmt.Errorf("link: Dial %s: %w", opts.URL, err)
	}

	// websocket.NetConn owns the websocket from this point — closing
	// the returned net.Conn cleanly closes the WS. The background
	// context is intentional: yamux will drive its own deadlines on
	// top of this connection.
	nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)

	sess, err := NewClientSession(nc)
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	return sess, nil
}
