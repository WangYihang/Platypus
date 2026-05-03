package main

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent"
	pluginrt "github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Stream adapter wiring: the 6 legacy stream handlers take typed
// proto requests (ProcessOpenRequest, FileReadRequest, ...) but the
// plugin runtime's StreamProvider interface takes raw metadata bytes
// (so it can stay schema-agnostic). The adapters here are the
// per-stream-type translation layer: unmarshal the bytes, dispatch
// to the typed handler.
//
// Names registered here MUST match the host_handler values in the
// system plugins' manifests (sys-streams). Renaming a provider
// requires bumping the matching system plugin to a new manifest +
// rebuilding/resigning its bundle.

// registerStreamProviders wires every legacy stream handler as a
// named StreamProvider on the plugin runtime. Called once at agent
// startup before EnsureInstalled so the bundled sys-streams plugin
// finds its handlers when the registry consults the claim list.
func registerStreamProviders(_ context.Context) {
	pluginrt.SetStreamProvider("agent.process",
		streamAdapter(agent.HandleProcessStream, func() proto.Message { return &v2pb.ProcessOpenRequest{} }))
	pluginrt.SetStreamProvider("agent.file_read",
		streamAdapter(agent.HandleFileReadStream, func() proto.Message { return &v2pb.FileReadRequest{} }))
	pluginrt.SetStreamProvider("agent.file_write",
		streamAdapter(agent.HandleFileWriteStream, func() proto.Message { return &v2pb.FileWriteRequest{} }))
	pluginrt.SetStreamProvider("agent.file_scan",
		streamAdapter(agent.HandleFileScanStream, func() proto.Message { return &v2pb.FileScanRequest{} }))
	pluginrt.SetStreamProvider("agent.file_archive",
		streamAdapter(agent.HandleFileArchiveStream, func() proto.Message { return &v2pb.FileArchiveRequest{} }))
	pluginrt.SetStreamProvider("agent.tunnel_pull",
		streamAdapter(agent.HandleTunnelPullStream, func() proto.Message { return &v2pb.TunnelPullRequest{} }))
}

// streamAdapter is the per-handler glue. The two type parameters
// would normally use generics but the legacy handler signatures
// vary (ProcessHandler vs FileReadHandler ...) — we'd end up with
// six slightly different generic functions. Plain typed wrappers
// per handler family are simpler and grep better; the closure
// captures the concrete handler.
func streamAdapter[T proto.Message](handler func(ctx context.Context, stream io.ReadWriteCloser, req T) error,
	newReq func() proto.Message) pluginrt.StreamProvider {
	return func(ctx context.Context, stream io.ReadWriteCloser, metadata []byte) error {
		req := newReq()
		if err := proto.Unmarshal(metadata, req); err != nil {
			return fmt.Errorf("stream adapter: unmarshal metadata: %w", err)
		}
		typed, ok := req.(T)
		if !ok {
			return fmt.Errorf("stream adapter: type assertion failed for %T", req)
		}
		return handler(ctx, stream, typed)
	}
}
