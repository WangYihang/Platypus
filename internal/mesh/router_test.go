package mesh

import (
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
	"testing"

	"google.golang.org/protobuf/proto"

)

// buildSignedLSA produces a properly-signed LSA for a fake origin
// so we can test Ingest + routing without standing up real Nodes.
// When origin has a CertPEM (the common case with the cert-bound
// mustIdentity helper) it's included in OriginCertPem so verifiers
// take the cert-bound identity check path.
func buildSignedLSA(t *testing.T, origin *Identity, seq uint64, links ...string) *v2pb.MeshLSA {
	t.Helper()
	linkMsgs := make([]*v2pb.MeshLSA_Link, 0, len(links))
	for _, l := range links {
		linkMsgs = append(linkMsgs, &v2pb.MeshLSA_Link{NodeId: l, Cost: 1})
	}
	lsa := &v2pb.MeshLSA{
		OriginNodeId:  origin.NodeID,
		Seq:           seq,
		Links:         linkMsgs,
		Pubkey:        origin.PublicKey,
		OriginCertPem: append([]byte(nil), origin.CertPEM...),
	}
	canon, err := proto.Marshal(lsa)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	lsa.Sig = signBytes(origin.PrivateKey, canon)
	return lsa
}

func TestLSDBIngestRejectsBadSignature(t *testing.T) {
	alice := mustIdentity(t)
	lsa := buildSignedLSA(t, alice, 1, "peer")
	// Tamper with links after signing.
	lsa.Links[0].NodeId = "attacker-inserted"
	db := newLSDB()
	if _, err := db.Ingest(lsa, nil); err == nil {
		t.Fatal("expected signature verification failure")
	}
}

func TestLSDBIngestRejectsPubkeyMismatch(t *testing.T) {
	alice := mustIdentity(t)
	bob := mustIdentity(t)
	lsa := buildSignedLSA(t, alice, 1, "peer")
	lsa.Pubkey = bob.PublicKey // Now origin NodeID doesn't match pubkey.
	db := newLSDB()
	if _, err := db.Ingest(lsa, nil); err == nil {
		t.Fatal("expected pubkey/origin mismatch to be rejected")
	}
}

func TestLSDBIngestIgnoresOlderSeq(t *testing.T) {
	alice := mustIdentity(t)
	db := newLSDB()
	if _, err := db.Ingest(buildSignedLSA(t, alice, 5, "p1"), nil); err != nil {
		t.Fatalf("ingest 5: %v", err)
	}
	changed, err := db.Ingest(buildSignedLSA(t, alice, 3, "p1"), nil)
	if err != nil {
		t.Fatalf("ingest 3: %v", err)
	}
	if changed {
		t.Fatal("expected older seq to be ignored")
	}
}

// TestRouteABVIATR exercises the routing math for a 3-node chain
// A -- B -- C. A can only reach C via B.
func TestComputeRoutesLinearChain(t *testing.T) {
	a := mustIdentity(t)
	b := mustIdentity(t)
	c := mustIdentity(t)

	lsdb := newLSDB()
	// B's LSA says B is connected to A and C.
	if _, err := lsdb.Ingest(buildSignedLSA(t, b, 1, a.NodeID, c.NodeID), nil); err != nil {
		t.Fatalf("ingest B: %v", err)
	}
	// C's LSA says C is connected to B.
	if _, err := lsdb.Ingest(buildSignedLSA(t, c, 1, b.NodeID), nil); err != nil {
		t.Fatalf("ingest C: %v", err)
	}

	// From A's perspective, its only direct peer is B.
	routes := computeRoutes(a.NodeID, lsdb, map[string]struct{}{b.NodeID: {}})
	if got, want := routes[c.NodeID], b.NodeID; got != want {
		t.Fatalf("route to C = %q, want %q", got, want)
	}
	if got, want := routes[b.NodeID], b.NodeID; got != want {
		t.Fatalf("route to B = %q, want %q (direct)", got, want)
	}
}

func TestComputeRoutesNoPath(t *testing.T) {
	a := mustIdentity(t)
	isolated := mustIdentity(t)
	lsdb := newLSDB()
	// A has no direct peers, LSDB is empty → no routes.
	routes := computeRoutes(a.NodeID, lsdb, nil)
	if _, ok := routes[isolated.NodeID]; ok {
		t.Fatal("expected no route to isolated node")
	}
}
