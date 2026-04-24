package mesh

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/flynn/noise"
	"google.golang.org/protobuf/proto"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// noiseCipherSuite is Noise_XXpsk3_25519_ChaChaPoly_BLAKE2b. Chosen
// to match WireGuard's primitive stack (same Curve + AEAD) so the
// security story is familiar to anyone who has audited a Noise-
// based VPN. XXpsk3 mixes the network-wide PSK at message 3 so
// membership still requires the shared secret, and both sides have
// authenticated each other via their static X25519 keys by the
// time the PSK contributes to the transcript.
var noiseCipherSuite = noise.NewCipherSuite(
	noise.DH25519, noise.CipherChaChaPoly, noise.HashBLAKE2b,
)

// noisePSKPlacement is the position at which the PSK is mixed into
// the handshake. 3 = at the start of message 3 (the initiator's
// final); chosen so the PSK contributes AFTER the responder's
// identity has been verified from the Noise transcript, avoiding
// the psk0/psk1 requirement that PSK be the first mixed secret.
const noisePSKPlacement = 3

// HandshakeResult is returned to both sides of a completed mesh
// handshake. It carries the peer's verified Ed25519 identity plus
// the addresses the peer advertises (used for gossip). The Noise
// session's CipherStates are discarded — mesh traffic rides over
// the outer TLS link in plaintext; Noise is used for mutual auth
// + PSK mixing only.
type HandshakeResult struct {
	PeerNodeID    string
	PeerPublicKey []byte // Ed25519 pubkey (from HandshakePayload)
	PeerAddresses []string
}

// newNoiseStaticKey wraps an Identity's derived X25519 keypair in
// noise.DHKey shape.
func newNoiseStaticKey(id *Identity) noise.DHKey {
	return noise.DHKey{
		Private: append([]byte(nil), id.X25519Private[:]...),
		Public:  append([]byte(nil), id.X25519Public[:]...),
	}
}

// runNoiseInitiator drives the XX handshake from the dialing side.
// Three messages: write(msg1) → read(msg2) → write(msg3). The
// responder's HandshakePayload is extracted from msg2 and returned
// as HandshakeResult; we send our own HandshakePayload in msg3 so
// the responder can record it too.
func runNoiseInitiator(
	ctx context.Context,
	codec *envCodec,
	id *Identity,
	psk []byte,
	ourPayload *v2pb.HandshakePayload,
) (*HandshakeResult, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:           noiseCipherSuite,
		Pattern:               noise.HandshakeXX,
		Initiator:             true,
		StaticKeypair:         newNoiseStaticKey(id),
		PresharedKey:          psk,
		PresharedKeyPlacement: noisePSKPlacement,
		Random:                rand.Reader,
	})
	if err != nil {
		return nil, fmt.Errorf("noise init: %w", err)
	}

	// -- msg 1: initiator → responder (ephemeral; no payload).
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg1: %w", err)
	}
	if err := sendWithCtx(ctx, codec, &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: nowNanos(),
		Payload:   &v2pb.MeshEnvelope_Hello{Hello: &v2pb.MeshHello{NoiseMsg: msg1}},
	}); err != nil {
		return nil, fmt.Errorf("send hello: %w", err)
	}

	// -- msg 2: responder → initiator (e, ee, s, es + payload).
	env, err := recvWithCtx(ctx, codec)
	if err != nil {
		return nil, fmt.Errorf("recv hello_ack: %w", err)
	}
	ack, ok := env.Payload.(*v2pb.MeshEnvelope_HelloAck)
	if !ok || ack.HelloAck == nil {
		return nil, errors.New("expected hello_ack payload")
	}
	peerPayloadBytes, _, _, err := hs.ReadMessage(nil, ack.HelloAck.NoiseMsg)
	if err != nil {
		return nil, fmt.Errorf("noise read msg2: %w", err)
	}
	peer, err := parseAndVerifyPayload(peerPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("hello_ack payload: %w", err)
	}
	if peer.PeerNodeID == id.NodeID {
		return nil, errors.New("peer claims own NodeID")
	}

	// -- msg 3: initiator → responder (s, se + PSK mix + our payload).
	ourPayloadBytes, err := proto.Marshal(ourPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	msg3, _, _, err := hs.WriteMessage(nil, ourPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("noise write msg3: %w", err)
	}
	if err := sendWithCtx(ctx, codec, &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: nowNanos(),
		Payload:   &v2pb.MeshEnvelope_HelloFinish{HelloFinish: &v2pb.MeshHelloFinish{NoiseMsg: msg3}},
	}); err != nil {
		return nil, fmt.Errorf("send hello_finish: %w", err)
	}

	return peer, nil
}

// runNoiseResponder drives the XX handshake from the listening side.
// Three messages: read(msg1) → write(msg2) → read(msg3). Our
// HandshakePayload goes out encrypted in msg2; the initiator's
// HandshakePayload arrives encrypted in msg3.
func runNoiseResponder(
	ctx context.Context,
	codec *envCodec,
	id *Identity,
	psk []byte,
	ourPayload *v2pb.HandshakePayload,
) (*HandshakeResult, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:           noiseCipherSuite,
		Pattern:               noise.HandshakeXX,
		Initiator:             false,
		StaticKeypair:         newNoiseStaticKey(id),
		PresharedKey:          psk,
		PresharedKeyPlacement: noisePSKPlacement,
		Random:                rand.Reader,
	})
	if err != nil {
		return nil, fmt.Errorf("noise init: %w", err)
	}

	// -- msg 1: recv initiator ephemeral.
	env, err := recvWithCtx(ctx, codec)
	if err != nil {
		return nil, fmt.Errorf("recv hello: %w", err)
	}
	hello, ok := env.Payload.(*v2pb.MeshEnvelope_Hello)
	if !ok || hello.Hello == nil {
		return nil, errors.New("expected hello payload")
	}
	if _, _, _, err := hs.ReadMessage(nil, hello.Hello.NoiseMsg); err != nil {
		return nil, fmt.Errorf("noise read msg1: %w", err)
	}

	// -- msg 2: send our ephemeral + static + payload.
	ourPayloadBytes, err := proto.Marshal(ourPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	msg2, _, _, err := hs.WriteMessage(nil, ourPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("noise write msg2: %w", err)
	}
	if err := sendWithCtx(ctx, codec, &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: nowNanos(),
		Payload:   &v2pb.MeshEnvelope_HelloAck{HelloAck: &v2pb.MeshHelloAck{NoiseMsg: msg2}},
	}); err != nil {
		return nil, fmt.Errorf("send hello_ack: %w", err)
	}

	// -- msg 3: recv initiator static + PSK mix + their payload.
	env, err = recvWithCtx(ctx, codec)
	if err != nil {
		return nil, fmt.Errorf("recv hello_finish: %w", err)
	}
	fin, ok := env.Payload.(*v2pb.MeshEnvelope_HelloFinish)
	if !ok || fin.HelloFinish == nil {
		return nil, errors.New("expected hello_finish payload")
	}
	peerPayloadBytes, _, _, err := hs.ReadMessage(nil, fin.HelloFinish.NoiseMsg)
	if err != nil {
		return nil, fmt.Errorf("noise read msg3: %w", err)
	}
	peer, err := parseAndVerifyPayload(peerPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("hello_finish payload: %w", err)
	}
	if peer.PeerNodeID == id.NodeID {
		return nil, errors.New("peer claims own NodeID")
	}
	return peer, nil
}

// parseAndVerifyPayload unmarshals a HandshakePayload and enforces
// cert-bound identity: the peer must ship a cert, the cert's SAN
// must match the claimed NodeID, and the cert's SPKI must match
// the claimed Ed25519 pubkey. Chain verification against the
// local TrustedCAs pool is the caller's job (see node.AcceptRaw /
// dialer.dial) so tests without a PKI can run the local check
// alone. Callers surface any error as an opaque "handshake
// failed" to the network — no leaky details.
func parseAndVerifyPayload(raw []byte) (*HandshakeResult, error) {
	var p v2pb.HandshakePayload
	if err := proto.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if p.Protocol != meshProtocolVersion {
		return nil, fmt.Errorf("unsupported protocol %d", p.Protocol)
	}
	if len(p.Ed25519Pubkey) != 32 {
		return nil, fmt.Errorf("bad ed25519 pubkey length %d", len(p.Ed25519Pubkey))
	}
	if p.NodeId == "" {
		return nil, errors.New("empty node_id")
	}
	if len(p.CertPem) == 0 {
		return nil, errors.New("empty cert_pem")
	}
	if err := verifyCertIdentityLocal(p.CertPem, p.Ed25519Pubkey, p.NodeId); err != nil {
		return nil, fmt.Errorf("handshake identity: %w", err)
	}
	return &HandshakeResult{
		PeerNodeID:    p.NodeId,
		PeerPublicKey: p.Ed25519Pubkey,
		PeerAddresses: p.Addresses,
	}, nil
}
