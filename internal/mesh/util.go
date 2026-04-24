package mesh

import (
	"crypto/ed25519"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// signBytes is a small wrapper kept separate so tests can stub it.
func signBytes(priv ed25519.PrivateKey, msg []byte) []byte {
	return ed25519.Sign(priv, msg)
}

// userHomeDir wraps os.UserHomeDir so tests can override it if needed.
var userHomeDir = func() (string, error) {
	return os.UserHomeDir()
}

// canonicalBytesForSig marshals m deterministically with proto's
// canonical encoding. Caller is expected to blank volatile fields
// (Sig, hop counters) on a cloned copy before invoking.
func canonicalBytesForSig(m proto.Message) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(m)
}

// signPeerAnnounce fills ann.Sig with an Ed25519 signature over the
// canonical bytes of the announce with Sig blanked. The caller
// populates OriginNodeId + Pubkey first.
func signPeerAnnounce(priv ed25519.PrivateKey, ann *v2pb.MeshPeerAnnounce) {
	ann.Sig = nil
	canon, err := canonicalBytesForSig(ann)
	if err != nil {
		return
	}
	ann.Sig = signBytes(priv, canon)
}

// verifyPeerAnnounce checks the Ed25519 sig against the claimed
// pubkey AND verifies the origin identity. When OriginCertPem is
// populated AND trustedCAs is non-nil, the cert is chain-verified
// against the pool and the SAN binding is enforced. Otherwise the
// verifier falls back to the legacy DeriveNodeID(pubkey) == origin
// self-cert check. A populated cert without a trusted pool is
// ignored (legacy path), not rejected — mesh-only tests work
// without a PKI configured.
func verifyPeerAnnounce(ann *v2pb.MeshPeerAnnounce, trustedCAs *x509.CertPool) error {
	if len(ann.Pubkey) != ed25519.PublicKeySize {
		return fmt.Errorf("peer_announce: bad pubkey length %d", len(ann.Pubkey))
	}
	if err := verifyGossipOrigin("peer_announce", ann.OriginCertPem, ann.Pubkey, ann.OriginNodeId, trustedCAs); err != nil {
		return err
	}
	sig := ann.Sig
	cp := proto.Clone(ann).(*v2pb.MeshPeerAnnounce)
	cp.Sig = nil
	canon, err := canonicalBytesForSig(cp)
	if err != nil {
		return fmt.Errorf("peer_announce: marshal: %w", err)
	}
	if !ed25519.Verify(ann.Pubkey, canon, sig) {
		return fmt.Errorf("peer_announce: bad signature")
	}
	return nil
}

// signPeerDelta signs delta with priv over the canonical bytes of
// the delta with Sig blanked AND Ttl blanked (Ttl decrements per
// hop, so it cannot be part of the signed payload).
func signPeerDelta(priv ed25519.PrivateKey, delta *v2pb.MeshPeerDelta) {
	delta.Sig = nil
	cp := proto.Clone(delta).(*v2pb.MeshPeerDelta)
	cp.Sig = nil
	cp.Ttl = 0
	canon, err := canonicalBytesForSig(cp)
	if err != nil {
		return
	}
	delta.Sig = signBytes(priv, canon)
}

// verifyPeerDelta is the ingest-side counterpart of signPeerDelta.
// See verifyPeerAnnounce for the cert-bound / legacy dispatch.
func verifyPeerDelta(delta *v2pb.MeshPeerDelta, trustedCAs *x509.CertPool) error {
	if len(delta.Pubkey) != ed25519.PublicKeySize {
		return fmt.Errorf("peer_delta: bad pubkey length %d", len(delta.Pubkey))
	}
	if err := verifyGossipOrigin("peer_delta", delta.OriginCertPem, delta.Pubkey, delta.OriginNodeId, trustedCAs); err != nil {
		return err
	}
	sig := delta.Sig
	cp := proto.Clone(delta).(*v2pb.MeshPeerDelta)
	cp.Sig = nil
	cp.Ttl = 0
	canon, err := canonicalBytesForSig(cp)
	if err != nil {
		return fmt.Errorf("peer_delta: marshal: %w", err)
	}
	if !ed25519.Verify(delta.Pubkey, canon, sig) {
		return fmt.Errorf("peer_delta: bad signature")
	}
	return nil
}

// verifyGossipOrigin picks the right identity check for a signed
// gossip payload. Three cases:
//
//  1. certPEM present + pool configured — full chain-verify
//     against pool + SAN/SPKI binding (cert-bound mode).
//  2. certPEM present + no pool — SAN/SPKI binding only (TOFU-
//     style: still strictly more evidence than the legacy hash
//     check, just without CA-chain anchoring). Lets nodes that
//     haven't been configured with a pool coexist in a cert-
//     bound mesh without hard-failing every inbound message.
//  3. No certPEM — legacy self-certifying DeriveNodeID check.
//
// prefix is purely for error-message readability.
func verifyGossipOrigin(prefix string, certPEM []byte, pubkey ed25519.PublicKey, nodeID string, trustedCAs *x509.CertPool) error {
	if len(certPEM) > 0 {
		if trustedCAs != nil {
			if err := verifyCertBoundIdentity(certPEM, trustedCAs, pubkey, nodeID); err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
			return nil
		}
		// Pool not configured locally; verify what we can without
		// it. This branch is the mixed-mode migration aid.
		if err := verifyCertIdentityLocal(certPEM, pubkey, nodeID); err != nil {
			return fmt.Errorf("%s: %w", prefix, err)
		}
		return nil
	}
	if DeriveNodeID(pubkey) != nodeID {
		return fmt.Errorf("%s: pubkey/origin mismatch", prefix)
	}
	return nil
}
