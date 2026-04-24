package mesh

import (
	"context"
	"errors"
	"fmt"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

const (
	meshProtocolVersion = 1
	handshakeTimeout    = 5 * time.Second
)

// ErrHandshake wraps any handshake failure so the caller can log a
// short reason without leaking whether it was the PSK, the signature,
// or a malformed frame that failed — attackers should see "handshake
// failed" and no more. Kept as a named type so call sites can still
// branch on it (e.g. Dialer backoff policy).
type ErrHandshake struct {
	Stage  string
	Detail string
}

func (e *ErrHandshake) Error() string {
	return fmt.Sprintf("mesh handshake: %s (%s)", e.Stage, e.Detail)
}

func handshakeError(stage, detail string) *ErrHandshake {
	return &ErrHandshake{Stage: stage, Detail: detail}
}

// nowNanos is a testing seam for the timestamp field in MeshEnvelope
// during the handshake frames. Plain time.Now() in production.
var nowNanos = func() int64 { return time.Now().UnixNano() }

// PerformClientHandshake runs the mesh handshake from the side that
// opened the transport (the "dialer" / "client" half). Three Noise
// XXpsk3 messages are exchanged; on success the peer's Ed25519
// identity is returned. PSK must be non-empty (≥16 bytes).
func PerformClientHandshake(
	ctx context.Context,
	codec *envCodec,
	id *Identity,
	psk []byte,
	advertisedAddrs []string,
) (*HandshakeResult, error) {
	if len(psk) < 16 {
		return nil, handshakeError("psk", "missing or too short")
	}
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	res, err := runNoiseInitiator(ctx, codec, id, psk, buildPayload(id, advertisedAddrs))
	return wrapNoiseErr(res, err)
}

// PerformServerHandshake runs the mesh handshake from the side that
// accepted the transport.
func PerformServerHandshake(
	ctx context.Context,
	codec *envCodec,
	id *Identity,
	psk []byte,
	advertisedAddrs []string,
) (*HandshakeResult, error) {
	if len(psk) < 16 {
		return nil, handshakeError("psk", "missing or too short")
	}
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	res, err := runNoiseResponder(ctx, codec, id, psk, buildPayload(id, advertisedAddrs))
	return wrapNoiseErr(res, err)
}

// buildPayload fills the HandshakePayload we advertise on our side
// of the Noise handshake. Kept local so the three callers stay in
// sync on field population. A cert-bound Identity always carries
// CertPEM; the payload publishes it so the peer can bind our
// Ed25519 identity to a project-CA-signed cert at handshake time.
func buildPayload(id *Identity, addresses []string) *v2pb.HandshakePayload {
	return &v2pb.HandshakePayload{
		NodeId:        id.NodeID,
		Ed25519Pubkey: append([]byte(nil), id.PublicKey...),
		Protocol:      meshProtocolVersion,
		Addresses:     append([]string(nil), addresses...),
		CertPem:       append([]byte(nil), id.CertPEM...),
	}
}

// wrapNoiseErr normalises runNoise{Initiator,Responder} errors into
// the ErrHandshake type the rest of the package (and its callers)
// expect. Classification is best-effort based on the error message;
// the Dialer only cares that *ErrHandshake is returned so it can
// apply a longer back-off for identity problems.
func wrapNoiseErr(res *HandshakeResult, err error) (*HandshakeResult, error) {
	if err == nil {
		return res, nil
	}
	var he *ErrHandshake
	if errors.As(err, &he) {
		return nil, he
	}
	stage := "noise"
	if isIDErr(err) {
		stage = "identity"
	}
	return nil, handshakeError(stage, err.Error())
}

func isIDErr(err error) bool {
	msg := err.Error()
	for _, probe := range []string{
		"node_id", "ed25519 pubkey", "peer claims", "unsupported protocol",
	} {
		if contains(msg, probe) {
			return true
		}
	}
	return false
}

// contains is a narrow strings.Contains shim so we don't import
// strings just for this helper (this file already pulls time + fmt).
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// sendWithCtx / recvWithCtx run a codec operation on a goroutine so
// the handshake honours context cancellation. envCodec itself blocks
// on its underlying ReadWriter, which doesn't accept a deadline, so
// we wrap it here.
func sendWithCtx(ctx context.Context, codec *envCodec, env *v2pb.MeshEnvelope) error {
	done := make(chan error, 1)
	go func() { done <- codec.Send(env) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func recvWithCtx(ctx context.Context, codec *envCodec) (*v2pb.MeshEnvelope, error) {
	type result struct {
		env *v2pb.MeshEnvelope
		err error
	}
	done := make(chan result, 1)
	go func() {
		env, err := codec.Recv()
		done <- result{env, err}
	}()
	select {
	case r := <-done:
		return r.env, r.err
	case <-ctx.Done():
		return nil, errors.New(ctx.Err().Error())
	}
}
