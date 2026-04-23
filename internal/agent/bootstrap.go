package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	mdnsService = "_platypus-mesh._tcp"
	mdnsDomain  = "local."
)

// AttemptNeighborBootstrap discovers a LAN neighbour, waits until the
// local mesh sees a reachable server node, then opens a routed mesh
// stream to that server and completes TLS over it.
func AttemptNeighborBootstrap(state *State, config *tls.Config) (*tls.Conn, error) {
	if state == nil || state.Mesh == nil {
		return nil, fmt.Errorf("mesh bootstrap unavailable: mesh node not running")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	seedDiscoveredPeers(ctx, state)

	serverNodeID, err := waitForBootstrapServer(ctx, state)
	if err != nil {
		return nil, err
	}

	raw, err := state.Mesh.DialBootstrap(ctx, serverNodeID)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(raw, config)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	return tlsConn, nil
}

func seedDiscoveredPeers(ctx context.Context, state *State) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		_ = resolver.Browse(ctx, mdnsService, mdnsDomain, entries)
	}()

	projectID := state.Mesh.ProjectID()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout.C:
			return
		case entry := <-entries:
			if entry == nil || entry.Instance == state.Mesh.NodeID() {
				continue
			}
			if !sameProject(entry.Text, projectID) {
				continue
			}
			addrs := discoveryAddrs(entry)
			if len(addrs) == 0 {
				continue
			}
			state.Mesh.EnsurePeer(context.Background(), entry.Instance, addrs)
		}
	}
}

func waitForBootstrapServer(ctx context.Context, state *State) (string, error) {
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		if nodeID, ok := state.Mesh.FindBootstrapServer(); ok {
			return nodeID, nil
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("mesh bootstrap timed out waiting for a reachable server")
		case <-tick.C:
		}
	}
}

func sameProject(txt []string, want string) bool {
	for _, t := range txt {
		if strings.HasPrefix(t, "project_id=") {
			return strings.TrimPrefix(t, "project_id=") == want
		}
	}
	return false
}

func discoveryAddrs(entry *zeroconf.ServiceEntry) []string {
	addrs := make([]string, 0, len(entry.AddrIPv4)+len(entry.AddrIPv6))
	for _, ip := range entry.AddrIPv4 {
		addrs = append(addrs, net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port)))
	}
	for _, ip := range entry.AddrIPv6 {
		addrs = append(addrs, net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port)))
	}
	return addrs
}
