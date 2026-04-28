package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// TestTopologyV1_EmptyProjectStillEmitsServerNode pins the contract
// the redesign locked in: the snapshot must always include a "self"
// mesh node even when no hosts have enrolled yet. A blank graph
// reads to operators as "the UI is broken" — they expect at least
// the controller they're talking to to show up.
func TestTopologyV1_EmptyProjectStillEmitsServerNode(t *testing.T) {
	a := newAgentRouteFixture(t, "topology-empty")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTopologyHandler(a.DB)
	RegisterV1TopologyRoutes(r, h, a.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// The fixture seeds one host under project a.ProjectID. To
	// exercise the "empty" case make a fresh project that has no
	// rows and hit topology against it.
	emptyProj := seedProjectForAPITest(t, a.DB, "topology-empty-proj",
		seedUserForAPITest(t, a.DB, "topology-empty-admin", user.RoleAdmin))
	url := srv.URL + "/api/v1/projects/" + emptyProj.ID + "/topology"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+a.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	var got struct {
		Machines  []map[string]any `json:"machines"`
		MeshNodes []map[string]any `json:"mesh_nodes"`
		Links     []map[string]any `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.MeshNodes) != 1 {
		t.Fatalf("mesh_nodes = %+v; want 1 (self)", got.MeshNodes)
	}
	if got.MeshNodes[0]["kind"] != "self" {
		t.Errorf("kind = %v; want \"self\"", got.MeshNodes[0]["kind"])
	}
	if len(got.Machines) != 0 {
		t.Errorf("machines = %+v; want 0", got.Machines)
	}
}

// TestTopologyV1_LiveAgentAddsMeshNodeAndLink covers the case the
// graph view actually exists for: an agent has enrolled AND its
// link.Session is currently registered. The snapshot must surface
// (1) the host as a machine compound, (2) an "agent" mesh node for
// it, and (3) a server↔agent link so the view actually shows
// reachability.
func TestTopologyV1_LiveAgentAddsMeshNodeAndLink(t *testing.T) {
	a := newAgentRouteFixture(t, "topology-live-agent")

	// Stand up a real link.Session pair so the live registry has
	// something genuine to return Get(agentID) for.
	svc := core.NewAgentLinkService()
	clientConn, serverConn := net.Pipe()
	serverCh := make(chan *link.Session, 1)
	go func() {
		s, err := link.NewServerSession(serverConn)
		if err != nil {
			t.Errorf("server session: %v", err)
			return
		}
		serverCh <- s
	}()
	agentSess, err := link.NewClientSession(clientConn)
	if err != nil {
		t.Fatalf("client session: %v", err)
	}
	peer := <-serverCh
	t.Cleanup(func() { agentSess.Close(); peer.Close() })
	svc.Register(a.AgentID, agentSess)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTopologyHandler(a.DB).WithAgentLinks(svc)
	RegisterV1TopologyRoutes(r, h, a.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Seed a live session for this host so the machine compound's
	// Sessions slice has at least one diamond. The fixture already
	// inserted the host row; look it up so the FK to hosts.id holds.
	host, err := a.DB.Hosts().GetByAgentID(context.Background(), a.AgentID)
	if err != nil {
		t.Fatalf("lookup host: %v", err)
	}
	if err := a.DB.Sessions().Insert(context.Background(), &storage.Session{
		ID:          "s-topology-live",
		ProjectID:   a.ProjectID,
		HostID:      host.ID,
		ConnectedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	url := srv.URL + "/api/v1/projects/" + a.ProjectID + "/topology"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+a.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	var got struct {
		Machines []struct {
			HostID string `json:"host_id"`
		} `json:"machines"`
		MeshNodes []struct {
			NodeID string `json:"node_id"`
			Kind   string `json:"kind"`
		} `json:"mesh_nodes"`
		Links []struct {
			A  string `json:"a"`
			B  string `json:"b"`
			Up bool   `json:"up"`
		} `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Machines) != 1 {
		t.Fatalf("machines = %+v; want 1", got.Machines)
	}
	// mesh_nodes contains "self" plus the live agent — order is not
	// guaranteed.
	hasSelf := false
	hasAgent := false
	for _, n := range got.MeshNodes {
		if n.Kind == "self" {
			hasSelf = true
		}
		if n.Kind == "agent" {
			hasAgent = true
		}
	}
	if !hasSelf || !hasAgent {
		t.Errorf("mesh_nodes = %+v; want both self and agent", got.MeshNodes)
	}
	if len(got.Links) != 1 || !got.Links[0].Up {
		t.Errorf("links = %+v; want one up link", got.Links)
	}
}
