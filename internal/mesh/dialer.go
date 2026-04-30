package mesh

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

func marshalEd25519PKCS8(priv ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("mesh: marshal PKCS8: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

const (
	dialTimeout    = 8 * time.Second
	minRetryDelay  = 5 * time.Second
	maxRetryDelay  = 5 * time.Minute
	redialGracePct = 10

	meshLinkPath = "/api/v1/mesh/link"
)

type Dialer struct {
	node   *Node
	logger *slog.Logger

	mu    sync.Mutex
	tasks map[string]*dialTask
}

type dialTask struct {
	nodeID  string
	addr    string
	backoff time.Duration
	stop    chan struct{}
}

func newDialer(node *Node) *Dialer {
	return &Dialer{
		node:   node,
		logger: node.logger,
		tasks:  map[string]*dialTask{},
	}
}

func (d *Dialer) EnsurePeer(ctx context.Context, nodeID string, addresses []string) {
	if nodeID == "" || nodeID == d.node.identity.NodeID {
		return
	}
	if d.node.hasLink(nodeID) {
		return
	}
	for _, addr := range addresses {
		if addr == "" {
			continue
		}
		key := dialKey(nodeID, addr)
		d.mu.Lock()
		if _, ok := d.tasks[key]; ok {
			d.mu.Unlock()
			continue
		}
		task := &dialTask{
			nodeID:  nodeID,
			addr:    addr,
			backoff: minRetryDelay,
			stop:    make(chan struct{}),
		}
		d.tasks[key] = task
		d.mu.Unlock()

		go d.run(ctx, task)
	}
}

func (d *Dialer) StopPeer(nodeID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, t := range d.tasks {
		if t.nodeID == nodeID {
			close(t.stop)
			delete(d.tasks, k)
		}
	}
}

func (d *Dialer) run(ctx context.Context, t *dialTask) {
	defer func() {
		d.mu.Lock()
		delete(d.tasks, dialKey(t.nodeID, t.addr))
		d.mu.Unlock()
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		if d.node.hasLink(t.nodeID) {
			return
		}
		err := d.dialOnce(ctx, t)
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		delay := jittered(t.backoff)
		d.logger.Debug("mesh dial failed, backing off",
			slog.String("peer", t.nodeID),
			slog.String("addr", t.addr),
			slog.Duration("backoff", delay),
			slog.String("error", err.Error()))
		select {
		case <-ctx.Done():
			return
		case <-t.stop:
			return
		case <-time.After(delay):
		}
		t.backoff *= 2
		if t.backoff > maxRetryDelay {
			t.backoff = maxRetryDelay
		}
	}
}

func (d *Dialer) dialOnce(ctx context.Context, t *dialTask) error {
	tlsCfg, captured, err := peerDialTLSConfig(d.node.identity, d.node.trustedCAs)
	if err != nil {
		return fmt.Errorf("mesh dial tls config: %w", err)
	}
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}
	wsURL := "wss://" + t.addr + meshLinkPath
	wsConn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPClient:   httpClient,
		Subprotocols: []string{LinkSubprotocol},
	})
	if err != nil {
		return fmt.Errorf("mesh ws dial: %w", err)
	}

	leaf := captured.Load()
	if leaf == nil {
		_ = wsConn.CloseNow()
		return errors.New("mesh dial: tls verify did not capture peer cert")
	}
	peerNodeID, err := meshNodeIDFromVerifiedCert(leaf)
	if err != nil {
		_ = wsConn.CloseNow()
		return err
	}
	if peerNodeID == d.node.identity.NodeID {
		_ = wsConn.CloseNow()
		return errors.New("mesh dial: peer is self")
	}
	if t.nodeID != "" && t.nodeID != peerNodeID {
		d.logger.Info("mesh dial: peer node_id mismatch",
			slog.String("want", t.nodeID),
			slog.String("got", peerNodeID))
	}
	peerPubkey, ok := leaf.PublicKey.(ed25519.PublicKey)
	if !ok {
		_ = wsConn.CloseNow()
		return errors.New("mesh dial: peer cert must use Ed25519 key")
	}
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})

	nc := NetConnFromWebSocket(context.Background(), wsConn)
	go d.node.AdoptStream(context.Background(), nc, peerNodeID, peerPubkey, leafPEM)
	return nil
}

// peerDialTLSConfig builds a tls.Config for outbound mesh dials. Uses
// the agent's project-CA-signed cert as client cert; verifies the
// peer's chain against the project CA but skips hostname matching
// (mesh peers carry only platypus:// URI SANs, not DNS / IP). The
// captured atomic.Pointer holds the verified peer cert post-handshake.
func peerDialTLSConfig(id *Identity, caPool *x509.CertPool) (*tls.Config, *atomic.Pointer[x509.Certificate], error) {
	if id == nil || len(id.CertPEM) == 0 || id.PrivateKey == nil {
		return nil, nil, errors.New("mesh: identity must carry CertPEM + PrivateKey")
	}
	if caPool == nil {
		return nil, nil, errors.New("mesh: trustedCAs required for mesh dial")
	}
	keyDER, err := marshalEd25519PKCS8(id.PrivateKey)
	if err != nil {
		return nil, nil, err
	}
	cert, err := tls.X509KeyPair(id.CertPEM, keyDER)
	if err != nil {
		return nil, nil, fmt.Errorf("mesh: tls X509KeyPair: %w", err)
	}
	captured := &atomic.Pointer[x509.Certificate]{}
	cfg := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, // hostname/IP match skipped; chain verified manually below
		MinVersion:         tls.VersionTLS12,
		NextProtos:         []string{"h2", "http/1.1"},
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("mesh: no peer cert")
			}
			leaf := cs.PeerCertificates[0]
			intermediates := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				intermediates.AddCert(c)
			}
			if _, err := leaf.Verify(x509.VerifyOptions{
				Roots:         caPool,
				Intermediates: intermediates,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			}); err != nil {
				return fmt.Errorf("mesh: peer chain verify: %w", err)
			}
			captured.Store(leaf)
			return nil
		},
	}
	return cfg, captured, nil
}

func meshNodeIDFromVerifiedCert(leaf *x509.Certificate) (string, error) {
	for _, u := range leaf.URIs {
		if u.Scheme != "platypus" {
			continue
		}
		id := strings.TrimPrefix(u.Path, "/")
		switch u.Host {
		case "agent":
			if id == "" {
				return "", errors.New("mesh: platypus://agent/ SAN has empty id")
			}
			return id, nil
		case "server":
			if id == "" {
				return "", errors.New("mesh: platypus://server/ SAN has empty id")
			}
			return "server-" + id, nil
		}
	}
	return "", errors.New("mesh: peer cert missing platypus:// URI SAN")
}

func dialKey(nodeID, addr string) string {
	return nodeID + "|" + addr
}

func jittered(d time.Duration) time.Duration {
	if d <= 0 {
		return minRetryDelay
	}
	jitter := (int64(d) / 100) * int64(redialGracePct)
	if jitter <= 0 {
		return d
	}
	offset := (time.Now().UnixNano() % (2 * jitter)) - jitter
	return d + time.Duration(offset)
}
