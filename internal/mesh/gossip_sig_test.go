package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestPeerAnnounce_SignAndVerify_RoundTrip is the happy path: a
// signPeerAnnounce round-trip passes verifyPeerAnnounce.
func TestPeerAnnounce_SignAndVerify_RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ann := &v2pb.MeshPeerAnnounce{
		OriginNodeId: DeriveNodeID(pub),
		Pubkey:       pub,
		Nodes: []*v2pb.NodeInfo{
			{NodeId: "peer1", Pubkey: pub, Addresses: []string{"1.2.3.4:9000"}},
		},
	}
	signPeerAnnounce(priv, ann)
	if err := verifyPeerAnnounce(ann, nil); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestPeerAnnounce_RejectsTampered flips a byte in the Nodes list
// after signing and expects verify to fail.
func TestPeerAnnounce_RejectsTampered(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ann := &v2pb.MeshPeerAnnounce{
		OriginNodeId: DeriveNodeID(pub),
		Pubkey:       pub,
		Nodes: []*v2pb.NodeInfo{
			{NodeId: "peer1", Pubkey: pub, Addresses: []string{"1.2.3.4:9000"}},
		},
	}
	signPeerAnnounce(priv, ann)
	// Sneak in an extra node post-signing.
	ann.Nodes = append(ann.Nodes, &v2pb.NodeInfo{NodeId: "forged"})
	if err := verifyPeerAnnounce(ann, nil); err == nil {
		t.Fatal("expected verify to fail on tampered announce")
	}
}

// TestPeerAnnounce_RejectsPubkeyOriginMismatch: an Announce whose
// pubkey doesn't hash to origin_node_id must be rejected even with
// a self-consistent signature.
func TestPeerAnnounce_RejectsPubkeyOriginMismatch(t *testing.T) {
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)
	ann := &v2pb.MeshPeerAnnounce{
		OriginNodeId: DeriveNodeID(pub2), // claims pub2's id
		Pubkey:       pub2,
	}
	// Sign with priv1, which doesn't match pub2.
	ann.Sig = nil
	canon, _ := canonicalBytesForSig(ann)
	ann.Sig = ed25519.Sign(priv1, canon)
	if err := verifyPeerAnnounce(ann, nil); err == nil {
		t.Fatal("expected verify to fail when sig doesn't match pubkey")
	}
}

// TestPeerDelta_SignAndVerify_RoundTrip covers the happy path.
// Importantly, the Ttl field is blanked during signing so a hop
// that decrements Ttl still yields a valid signature.
func TestPeerDelta_SignAndVerify_RoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	delta := &v2pb.MeshPeerDelta{
		OriginNodeId: DeriveNodeID(pub),
		Pubkey:       pub,
		Seq:          1,
		Ttl:          5,
		Added:        []*v2pb.NodeInfo{{NodeId: "x", Pubkey: pub}},
	}
	signPeerDelta(priv, delta)
	// Simulate a flood hop decrementing TTL.
	delta.Ttl--
	if err := verifyPeerDelta(delta, nil); err != nil {
		t.Fatalf("verify after hop: %v", err)
	}
}

// TestPeerDelta_RejectsTamperedAdded confirms that modifying Added
// after signing invalidates the delta.
func TestPeerDelta_RejectsTamperedAdded(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	delta := &v2pb.MeshPeerDelta{
		OriginNodeId: DeriveNodeID(pub),
		Pubkey:       pub,
		Seq:          1,
		Ttl:          5,
		Added:        []*v2pb.NodeInfo{{NodeId: "x", Pubkey: pub}},
	}
	signPeerDelta(priv, delta)
	delta.Added = append(delta.Added, &v2pb.NodeInfo{NodeId: "forged"})
	if err := verifyPeerDelta(delta, nil); err == nil {
		t.Fatal("expected verify to fail on tampered delta")
	}
}

// TestPeerDelta_RejectsBadPubkeyLength guards the pubkey-length
// precondition on the verify path.
func TestPeerDelta_RejectsBadPubkeyLength(t *testing.T) {
	delta := &v2pb.MeshPeerDelta{
		OriginNodeId: "short-id",
		Pubkey:       []byte("not-32-bytes"),
	}
	if err := verifyPeerDelta(delta, nil); err == nil {
		t.Fatal("expected verify to reject bad pubkey length")
	}
}
