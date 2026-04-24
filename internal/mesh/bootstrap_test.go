package mesh

import (
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
	"log/slog"
	"net"
	"testing"
	"time"

)

func TestRegistryToNodeInfosIncludesBootstrapMetadata(t *testing.T) {
	r := newRegistry()
	id := mustIdentity(t)
	rec := &PeerRecord{
		NodeID:           id.NodeID,
		PublicKey:        id.PublicKey,
		Addresses:        []string{"127.0.0.1:7001"},
		LastSeen:         time.Unix(123, 0),
		Role:             "server",
		BootstrapService: true,
	}
	if !r.Upsert(rec) {
		t.Fatal("expected record upsert to change registry")
	}

	infos := r.ToNodeInfos()
	if len(infos) != 1 {
		t.Fatalf("got %d node infos, want 1", len(infos))
	}
	if infos[0].Role != "server" || !infos[0].BootstrapService {
		t.Fatalf("missing bootstrap metadata in node info: %+v", infos[0])
	}
}

func TestFindBootstrapServerPrefersReachableServer(t *testing.T) {
	id := mustIdentity(t)
	node, err := NewNode(Config{
		Identity:  id,
		PSK:       randomPSK(t),
		Role:      "agent",
		ProjectID: "proj",
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	serverID := mustIdentity(t)
	if !node.registry.Upsert(&PeerRecord{
		NodeID:           serverID.NodeID,
		PublicKey:        serverID.PublicKey,
		Addresses:        []string{"127.0.0.1:7443"},
		LastSeen:         time.Now(),
		Role:             "server",
		BootstrapService: true,
	}) {
		t.Fatal("expected server record to be inserted")
	}
	node.routes.Replace(map[string]string{serverID.NodeID: "next-hop"})

	got, ok := node.FindBootstrapServer()
	if !ok {
		t.Fatal("expected reachable bootstrap server")
	}
	if got != serverID.NodeID {
		t.Fatalf("FindBootstrapServer = %q, want %q", got, serverID.NodeID)
	}
}

func TestAdoptLinkPreservesBootstrapMetadata(t *testing.T) {
	id := mustIdentity(t)
	node, err := NewNode(Config{
		Identity:  id,
		PSK:       randomPSK(t),
		Role:      "agent",
		ProjectID: "proj",
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	peer := mustIdentity(t)
	if !node.registry.Upsert(&PeerRecord{
		NodeID:           peer.NodeID,
		PublicKey:        peer.PublicKey,
		Addresses:        []string{"10.0.0.1:7001"},
		LastSeen:         time.Now().Add(-time.Minute),
		Role:             "server",
		BootstrapService: true,
	}) {
		t.Fatal("expected existing peer record")
	}

	if !node.adoptLink(&Link{PeerNodeID: peer.NodeID, PeerPublicKey: peer.PublicKey, PeerAddresses: []string{"10.0.0.2:7001"}}) {
		t.Fatal("expected adoptLink to accept peer")
	}

	got := node.registry.Get(peer.NodeID)
	if got == nil {
		t.Fatal("registry lost peer")
	}
	if got.Role != "server" || !got.BootstrapService {
		t.Fatalf("bootstrap metadata was cleared: %+v", got)
	}
	if len(got.Addresses) != 2 {
		t.Fatalf("addresses = %#v, want both existing and direct-link addresses", got.Addresses)
	}
}

func TestMeshStreamHandleDataDoesNotBlockLinkLoop(t *testing.T) {
	id := mustIdentity(t)
	node, err := NewNode(Config{
		Identity:  id,
		PSK:       randomPSK(t),
		Role:      "agent",
		ProjectID: "proj",
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	streamConn, blockedPeer := net.Pipe()
	defer blockedPeer.Close()
	st := node.streams.newState(streamKey{initiator: "peer", id: 1}, "peer", streamConn, false)
	defer node.streams.closeStream(st, "test done", false)

	done := make(chan struct{})
	go func() {
		node.streams.handleData(&v2pb.MeshEnvelope{
			Payload: &v2pb.MeshEnvelope_StreamData{StreamData: &v2pb.MeshStreamData{
				InitiatorNodeId: "peer",
				StreamId:        1,
				Chunk:           []byte("hello"),
			}},
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("handleData blocked on slow stream consumer")
	}
}
