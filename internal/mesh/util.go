package mesh

import (
	"crypto/ed25519"
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

// verifyPeerAnnounce checks the sig against the claimed pubkey +
// confirms pubkey hashes to origin_node_id. Returns nil on success.
func verifyPeerAnnounce(ann *v2pb.MeshPeerAnnounce) error {
	if len(ann.Pubkey) != ed25519.PublicKeySize {
		return fmt.Errorf("peer_announce: bad pubkey length %d", len(ann.Pubkey))
	}
	if DeriveNodeID(ann.Pubkey) != ann.OriginNodeId {
		return fmt.Errorf("peer_announce: pubkey/origin mismatch")
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
func verifyPeerDelta(delta *v2pb.MeshPeerDelta) error {
	if len(delta.Pubkey) != ed25519.PublicKeySize {
		return fmt.Errorf("peer_delta: bad pubkey length %d", len(delta.Pubkey))
	}
	if DeriveNodeID(delta.Pubkey) != delta.OriginNodeId {
		return fmt.Errorf("peer_delta: pubkey/origin mismatch")
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
