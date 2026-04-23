package mesh

import (
	"log/slog"
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
