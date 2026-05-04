// fakeplugin is the minimal PluginRuntime implementation used by the
// process runtime's tests. Behaviour is controlled by the
// FAKE_BEHAVIOR env var:
//
//	(unset)            - normal: print READY, serve Health/Shutdown
//	"no_ready"         - skip the READY line; tests launch timeout
//	"slow_ready"       - delay READY by 4s; tests launch timeout
//	"id_mismatch"      - report a different plugin id; tests handshake
//	"version_mismatch" - report a different version
//	"unhealthy"        - Health returns ready=false forever
//	"ignore_shutdown"  - block Shutdown forever; tests SIGTERM/SIGKILL
//	"crash_on_start"   - exit 17 immediately
//	"crash_after_ready"- print READY then exit 23
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

type srv struct {
	v2pb.UnimplementedPluginRuntimeServer
	id      string
	version string
	beh     string
	server  *grpc.Server
}

func (s *srv) Health(ctx context.Context, _ *emptypb.Empty) (*v2pb.PluginHealthResponse, error) {
	id := s.id
	ver := s.version
	switch s.beh {
	case "id_mismatch":
		id = "wrong-id"
	case "version_mismatch":
		ver = "9.9.9"
	}
	return &v2pb.PluginHealthResponse{
		PluginId: id,
		Version:  ver,
		Ready:    s.beh != "unhealthy",
	}, nil
}

func (s *srv) Shutdown(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if s.beh == "ignore_shutdown" {
		// Block until killed.
		<-ctx.Done()
		return &emptypb.Empty{}, nil
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.server.GracefulStop()
	}()
	return &emptypb.Empty{}, nil
}

func main() {
	beh := os.Getenv("FAKE_BEHAVIOR")
	if beh == "crash_on_start" {
		os.Exit(17)
	}

	sock := os.Getenv("PLATYPUS_PLUGIN_SOCKET")
	id := os.Getenv("PLATYPUS_PLUGIN_ID")
	version := os.Getenv("PLATYPUS_PLUGIN_VERSION")
	if sock == "" {
		fmt.Fprintln(os.Stderr, "PLATYPUS_PLUGIN_SOCKET not set")
		os.Exit(2)
	}

	lis, err := net.Listen("unix", sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(3)
	}

	gs := grpc.NewServer()
	s := &srv{id: id, version: version, beh: beh, server: gs}
	v2pb.RegisterPluginRuntimeServer(gs, s)

	switch beh {
	case "no_ready":
		// skip
	case "slow_ready":
		go func() {
			time.Sleep(4 * time.Second)
			fmt.Println("READY")
		}()
	default:
		fmt.Println("READY")
	}

	if beh == "crash_after_ready" {
		os.Exit(23)
	}

	// Reap SIGTERM as a graceful exit so tests that escalate past
	// the gRPC Shutdown() path still see exit-0 once the signal
	// arrives — except for ignore_shutdown which keeps blocking.
	if beh != "ignore_shutdown" {
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGTERM)
			<-sigCh
			gs.GracefulStop()
		}()
	}

	if err := gs.Serve(lis); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(4)
	}
}
