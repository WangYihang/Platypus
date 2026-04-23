package core

import (
	"fmt"
	"net"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/protocol"
	"github.com/WangYihang/Platypus/internal/utils/update"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

func TestGatherClientInfoIgnoresEarlySysInfoResponse(t *testing.T) {
	serverConn, agentConn := net.Pipe()
	defer serverConn.Close()
	defer agentConn.Close()

	client := NewAgentClientForTest(serverConn)
	codec := protocol.NewProtoCodec(agentConn)

	done := make(chan error, 1)
	go func() {
		req, err := codec.Recv()
		if err != nil {
			done <- err
			return
		}
		if req.GetGetClientInfoRequest() == nil {
			done <- fmt.Errorf("unexpected payload: %T", req.Payload)
			return
		}
		if err := codec.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_SysInfoResponse{SysInfoResponse: &agentpb.SysInfoResponse{}},
		}); err != nil {
			done <- err
			return
		}
		if err := codec.Send(&agentpb.Envelope{
			RequestId: req.RequestId,
			Payload: &agentpb.Envelope_ClientInfoResponse{ClientInfoResponse: &agentpb.ClientInfoResponse{
				Version:           update.Version,
				Os:                runtime.GOOS,
				User:              "tester",
				Hostname:          "host-1",
				NetworkInterfaces: map[string]string{"eth0": "aa:bb:cc:dd:ee:ff"},
				MachineId:         "machine-1",
			}},
		}); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	if !client.GatherClientInfo("%u") {
		t.Fatal("GatherClientInfo returned false")
	}
	if err := <-done; err != nil {
		t.Fatalf("agent side exchange failed: %v", err)
	}
	if client.User != "tester" || client.Hostname != "host-1" || client.MachineID != "machine-1" {
		t.Fatalf("client info not populated: %+v", client)
	}
}
