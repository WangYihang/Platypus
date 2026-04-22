package mesh

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/WangYihang/Platypus/internal/protocol"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

const (
	meshProtocolVersion = 1
	pskDomainHello      = "platypus-mesh-hello"
	pskDomainHelloAck   = "platypus-mesh-hello-ack"
	handshakeTimeout    = 5 * time.Second
)

// ErrHandshake wraps any handshake failure so the caller can log a short
// reason without leaking whether it was the PSK, the signature, or a
// malformed frame that failed — attackers should see "handshake failed"
// and no more.
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

// HandshakeResult is returned to both sides of a completed mesh
// handshake. It carries the peer's verified identity plus the addresses
// the peer advertises (used for gossip).
type HandshakeResult struct {
	PeerNodeID    string
	PeerPublicKey ed25519.PublicKey
	PeerAddresses []string
}

// pskMAC computes HMAC-SHA256(psk, domain || pubkey || nonce).
func pskMAC(psk []byte, domain string, pubkey, nonce []byte) []byte {
	mac := hmac.New(sha256.New, psk)
	mac.Write([]byte(domain))
	mac.Write(pubkey)
	mac.Write(nonce)
	return mac.Sum(nil)
}

// sigMessage is the canonical bytestring the responder signs: peer's
// hello nonce, then own ack nonce, then peer's claimed node_id. Including
// the peer node_id prevents an attacker who captures a valid Ack from
// reusing it against a different initiator.
func sigMessage(peerNonce, ownNonce []byte, peerNodeID string) []byte {
	buf := make([]byte, 0, len(peerNonce)+len(ownNonce)+len(peerNodeID))
	buf = append(buf, peerNonce...)
	buf = append(buf, ownNonce...)
	buf = append(buf, []byte(peerNodeID)...)
	return buf
}

// validateHelloCommon checks invariants shared between Hello and HelloAck
// on the receiving side. It does NOT check the signature (HelloAck only).
func validateHelloCommon(
	psk []byte,
	domain string,
	nodeID string,
	pubkey, nonce, mac []byte,
	protocolVer uint32,
) error {
	if protocolVer != meshProtocolVersion {
		return handshakeError("protocol", fmt.Sprintf("unsupported protocol %d", protocolVer))
	}
	if len(pubkey) != ed25519.PublicKeySize {
		return handshakeError("pubkey", "wrong length")
	}
	if len(nonce) != 32 {
		return handshakeError("nonce", "wrong length")
	}
	if DeriveNodeID(pubkey) != nodeID {
		return handshakeError("node_id", "does not match pubkey")
	}
	expected := pskMAC(psk, domain, pubkey, nonce)
	if subtle.ConstantTimeCompare(expected, mac) != 1 {
		return handshakeError("psk_mac", "mismatch")
	}
	return nil
}

// PerformClientHandshake runs the mesh handshake from the side that
// opened the transport (the "dialer" / "client" half). It sends
// MeshHello, expects MeshHelloAck, verifies the MAC + signature, and
// returns the peer's identity.
func PerformClientHandshake(
	ctx context.Context,
	codec *protocol.ProtoCodec,
	id *Identity,
	psk []byte,
	advertisedAddrs []string,
) (*HandshakeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	helloNonce := make([]byte, 32)
	if _, err := rand.Read(helloNonce); err != nil {
		return nil, handshakeError("nonce_gen", err.Error())
	}

	hello := &agentpb.MeshHello{
		NodeId:    id.NodeID,
		Pubkey:    id.PublicKey,
		Nonce:     helloNonce,
		PskMac:    pskMAC(psk, pskDomainHello, id.PublicKey, helloNonce),
		Protocol:  meshProtocolVersion,
		Addresses: advertisedAddrs,
	}
	if err := sendWithCtx(ctx, codec, &agentpb.Envelope{
		Timestamp: time.Now().UnixNano(),
		Version:   meshProtocolVersion,
		Payload:   &agentpb.Envelope_MeshHello{MeshHello: hello},
	}); err != nil {
		return nil, handshakeError("send_hello", err.Error())
	}

	env, err := recvWithCtx(ctx, codec)
	if err != nil {
		return nil, handshakeError("recv_ack", err.Error())
	}
	ack, ok := env.Payload.(*agentpb.Envelope_MeshHelloAck)
	if !ok {
		return nil, handshakeError("recv_ack", "unexpected payload")
	}
	a := ack.MeshHelloAck
	if err := validateHelloCommon(psk, pskDomainHelloAck, a.NodeId, a.Pubkey, a.Nonce, a.PskMac, a.Protocol); err != nil {
		return nil, err
	}
	if !ed25519.Verify(a.Pubkey, sigMessage(helloNonce, a.Nonce, id.NodeID), a.Sig) {
		return nil, handshakeError("signature", "invalid")
	}
	return &HandshakeResult{
		PeerNodeID:    a.NodeId,
		PeerPublicKey: a.Pubkey,
		PeerAddresses: a.Addresses,
	}, nil
}

// PerformServerHandshake runs the mesh handshake from the side that
// accepted the transport (the "listener" / "server" half). It waits for
// MeshHello, verifies it, signs a challenge, and replies with
// MeshHelloAck.
func PerformServerHandshake(
	ctx context.Context,
	codec *protocol.ProtoCodec,
	id *Identity,
	psk []byte,
	advertisedAddrs []string,
) (*HandshakeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	env, err := recvWithCtx(ctx, codec)
	if err != nil {
		return nil, handshakeError("recv_hello", err.Error())
	}
	hello, ok := env.Payload.(*agentpb.Envelope_MeshHello)
	if !ok {
		return nil, handshakeError("recv_hello", "unexpected payload")
	}
	h := hello.MeshHello
	if err := validateHelloCommon(psk, pskDomainHello, h.NodeId, h.Pubkey, h.Nonce, h.PskMac, h.Protocol); err != nil {
		return nil, err
	}
	if h.NodeId == id.NodeID {
		return nil, handshakeError("node_id", "peer claims own NodeID")
	}

	ackNonce := make([]byte, 32)
	if _, err := rand.Read(ackNonce); err != nil {
		return nil, handshakeError("nonce_gen", err.Error())
	}
	sig := ed25519.Sign(id.PrivateKey, sigMessage(h.Nonce, ackNonce, h.NodeId))
	ack := &agentpb.MeshHelloAck{
		NodeId:    id.NodeID,
		Pubkey:    id.PublicKey,
		Nonce:     ackNonce,
		PskMac:    pskMAC(psk, pskDomainHelloAck, id.PublicKey, ackNonce),
		Sig:       sig,
		Protocol:  meshProtocolVersion,
		Addresses: advertisedAddrs,
	}
	if err := sendWithCtx(ctx, codec, &agentpb.Envelope{
		Timestamp: time.Now().UnixNano(),
		Version:   meshProtocolVersion,
		Payload:   &agentpb.Envelope_MeshHelloAck{MeshHelloAck: ack},
	}); err != nil {
		return nil, handshakeError("send_ack", err.Error())
	}
	return &HandshakeResult{
		PeerNodeID:    h.NodeId,
		PeerPublicKey: h.Pubkey,
		PeerAddresses: h.Addresses,
	}, nil
}

// sendWithCtx / recvWithCtx run a codec operation on a goroutine so the
// handshake honours context cancellation. The ProtoCodec itself blocks
// on its underlying ReadWriter, which doesn't accept a deadline, so we
// wrap it.
func sendWithCtx(ctx context.Context, codec *protocol.ProtoCodec, env *agentpb.Envelope) error {
	done := make(chan error, 1)
	go func() { done <- codec.Send(env) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func recvWithCtx(ctx context.Context, codec *protocol.ProtoCodec) (*agentpb.Envelope, error) {
	type result struct {
		env *agentpb.Envelope
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
