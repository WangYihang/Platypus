package mesh

import (
	"bytes"
	"testing"
	"time"
)

// TestRegistry_CertBoundPeerRoundTrips feeds a cert-bearing peer
// through Upsert → Snapshot → ToNodeInfos, asserts the cert bytes
// survive every stage, and that identity consistency (SAN ↔
// node_id, cert pubkey ↔ record pubkey) is enforced.
func TestRegistry_CertBoundPeerRoundTrips(t *testing.T) {
	certPEM, keyPEM := mintLeafPair(t, "agent-cert-rt", "proj-rt")
	id, err := LoadIdentityFromCert(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIdentityFromCert: %v", err)
	}

	r := newRegistry()
	rec := &PeerRecord{
		NodeID:    id.NodeID,
		PublicKey: id.PublicKey,
		CertPEM:   id.CertPEM,
		LastSeen:  time.Now(),
		Addresses: []string{"1.2.3.4:9000"},
		Role:      "agent",
	}
	if !r.Upsert(rec) {
		t.Fatal("Upsert rejected a valid cert-bound PeerRecord")
	}

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d; want 1", len(snap))
	}
	got := snap[0]
	if got.NodeID != id.NodeID {
		t.Fatalf("NodeID = %q; want %q", got.NodeID, id.NodeID)
	}
	if !bytes.Equal(got.CertPEM, certPEM) {
		t.Fatal("CertPEM not preserved through Upsert+Snapshot")
	}
	if &got.CertPEM[0] == &rec.CertPEM[0] {
		t.Fatal("CertPEM is aliased to the input slice; copyPeer should clone")
	}

	infos := r.ToNodeInfos()
	if len(infos) != 1 {
		t.Fatalf("ToNodeInfos len = %d; want 1", len(infos))
	}
	if !bytes.Equal(infos[0].CertPem, certPEM) {
		t.Fatal("CertPem not carried on the wire")
	}
}

// TestRegistry_RejectsCertNodeIDMismatch catches an attacker
// announcing a cert-bearing record where cert SAN id != claimed
// node_id. Without this, a compromised peer could re-badge any
// cert it holds under a different NodeID during gossip.
func TestRegistry_RejectsCertNodeIDMismatch(t *testing.T) {
	certPEM, keyPEM := mintLeafPair(t, "agent-real", "proj-x")
	id, err := LoadIdentityFromCert(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIdentityFromCert: %v", err)
	}

	r := newRegistry()
	rec := &PeerRecord{
		NodeID:    "totally-different-id",
		PublicKey: id.PublicKey,
		CertPEM:   certPEM,
		LastSeen:  time.Now(),
	}
	if r.Upsert(rec) {
		t.Fatal("Upsert should reject when cert SAN != claimed NodeID")
	}
}

// TestRegistry_RejectsCertPubkeyMismatch catches the other forgery
// vector: an attacker announces a legit cert but swaps in a pubkey
// they control. The cert's SPKI check must reject.
func TestRegistry_RejectsCertPubkeyMismatch(t *testing.T) {
	certPEM, _ := mintLeafPair(t, "agent-real", "proj-y")
	otherCertPEM, otherKeyPEM := mintLeafPair(t, "agent-other", "proj-y")
	otherID, err := LoadIdentityFromCert(otherCertPEM, otherKeyPEM)
	if err != nil {
		t.Fatalf("LoadIdentityFromCert (other): %v", err)
	}

	cert, err := parseAgentLeafCert(certPEM)
	if err != nil {
		t.Fatalf("parse real cert: %v", err)
	}
	agentID, _ := agentIDFromCert(cert)

	r := newRegistry()
	// Declare the real cert's node_id but the other key's pubkey.
	rec := &PeerRecord{
		NodeID:    agentID,
		PublicKey: otherID.PublicKey,
		CertPEM:   certPEM,
		LastSeen:  time.Now(),
	}
	if r.Upsert(rec) {
		t.Fatal("Upsert should reject when record pubkey != cert pubkey")
	}
}

// TestRegistry_RejectsRecordWithoutCert: cert-binding is the only
// accepted mode; a PeerRecord without CertPEM must be refused.
func TestRegistry_RejectsRecordWithoutCert(t *testing.T) {
	id := mustIdentity(t) // cert-bound
	r := newRegistry()
	rec := &PeerRecord{
		NodeID:    id.NodeID,
		PublicKey: id.PublicKey,
		// CertPEM intentionally empty
		LastSeen: time.Now(),
	}
	if r.Upsert(rec) {
		t.Fatal("Upsert should reject a record without CertPEM")
	}
}
