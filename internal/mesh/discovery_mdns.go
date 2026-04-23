package mesh

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
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

func (n *Node) runDiscovery(ctx context.Context) {
	if !n.cfg.DiscoveryLAN {
		return
	}

	// We can only advertise if we have a listener.
	// If ListenAddr is ":0", ListenerAddr() will return the actual bound port
	// after ln.Serve starts. But n.Start calls ln.Serve in a goroutine.
	// We should wait a bit or use a more robust way.
	go func() {
		// Wait up to 5 seconds for the listener to be ready.
		for i := 0; i < 50; i++ {
			addr := n.ListenerAddr()
			if addr != "" {
				n.advertise(ctx, addr)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
		n.logger.Warn("mdns advertise: listener not ready after 5s, discovery disabled")
	}()

	go n.browse(ctx)
}

func (n *Node) advertise(ctx context.Context, addr string) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		n.logger.Error("mdns advertise: split host port", slog.String("error", err.Error()), slog.String("addr", addr))
		return
	}
	port, _ := strconv.Atoi(portStr)

	txt := []string{
		fmt.Sprintf("project_id=%s", n.cfg.ProjectID),
	}

	server, err := zeroconf.Register(n.NodeID(), mdnsService, mdnsDomain, port, txt, nil)
	if err != nil {
		n.logger.Error("mdns register", slog.String("error", err.Error()))
		return
	}
	defer server.Shutdown()

	n.logger.Info("mdns advertising", slog.Int("port", port), slog.String("project_id", n.cfg.ProjectID))

	<-ctx.Done()
}

func (n *Node) browse(ctx context.Context) {
	interval := time.Duration(n.cfg.DiscoveryInterval) * time.Second
	if interval < 10*time.Second {
		interval = 30 * time.Second
	}

	n.logger.Info("mdns browsing started", slog.Duration("interval", interval))

	// Initial scan
	n.doBrowse(ctx)

	for {
		// Add up to 20% jitter to the interval
		jitter := time.Duration(float64(interval) * 0.2 * (2.0*rand.Float64() - 1.0))
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval + jitter):
			n.doBrowse(ctx)
		}
	}
}

func (n *Node) doBrowse(ctx context.Context) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		n.logger.Error("mdns resolver", slog.String("error", err.Error()))
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		err := resolver.Browse(ctx, mdnsService, mdnsDomain, entries)
		if err != nil {
			n.logger.Error("mdns browse", slog.String("error", err.Error()))
		}
	}()

	// Give it some time to collect results (mDNS is somewhat asynchronous)
	timeout := time.After(5 * time.Second)

loop:
	for {
		select {
		case entry := <-entries:
			if entry == nil {
				break loop
			}
			n.handleDiscoveryEntry(entry)
		case <-timeout:
			break loop
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) handleDiscoveryEntry(entry *zeroconf.ServiceEntry) {
	if entry.Instance == n.NodeID() {
		return
	}

	// Parse TXT records for ProjectID
	var projectID string
	for _, t := range entry.Text {
		if strings.HasPrefix(t, "project_id=") {
			projectID = strings.TrimPrefix(t, "project_id=")
			break
		}
	}

	if projectID != n.cfg.ProjectID {
		return
	}

	// We found a peer in the same project!
	var addrs []string
	for _, ip := range entry.AddrIPv4 {
		addrs = append(addrs, net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port)))
	}
	for _, ip := range entry.AddrIPv6 {
		addrs = append(addrs, net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port)))
	}

	if len(addrs) == 0 {
		return
	}

	// If we don't have a link yet, tell the dialer to ensure we have one.
	if !n.hasLink(entry.Instance) {
		n.logger.Info("discovered lan peer", slog.String("peer", entry.Instance), slog.Any("addrs", addrs))
		n.dialer.EnsurePeer(context.Background(), entry.Instance, addrs)
	}
}
