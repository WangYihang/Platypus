package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestFanOutRPC_AllOK fans out a trivial RPC across N agents using a
// stub dispatcher. Asserts every agent gets its expected response,
// the order in the result slice matches the input order, and the
// total number of dispatcher calls equals the input size.
func TestFanOutRPC_AllOK(t *testing.T) {
	agentIDs := []string{"agent-a", "agent-b", "agent-c"}
	var calls atomic.Int64
	disp := func(_ context.Context, id string, _ *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
		calls.Add(1)
		return &v2pb.RpcResponse{Error: "echo:" + id}, nil
	}

	got := FanOutRPC(context.Background(), agentIDs, &v2pb.RpcRequest{}, FanOutOptions{
		MaxConcurrency: 2,
	}, disp)

	if calls.Load() != int64(len(agentIDs)) {
		t.Errorf("dispatcher calls = %d, want %d", calls.Load(), len(agentIDs))
	}
	if len(got) != len(agentIDs) {
		t.Fatalf("results = %d, want %d", len(got), len(agentIDs))
	}
	for i, r := range got {
		if r.AgentID != agentIDs[i] {
			t.Errorf("result[%d].AgentID = %q, want %q (order must match input)", i, r.AgentID, agentIDs[i])
		}
		if r.Err != nil {
			t.Errorf("result[%d].Err = %v, want nil", i, r.Err)
		}
		if r.Resp == nil {
			t.Errorf("result[%d].Resp = nil", i)
			continue
		}
		want := "echo:" + agentIDs[i]
		if r.Resp.GetError() != want {
			t.Errorf("result[%d].Resp.Error = %q, want %q", i, r.Resp.GetError(), want)
		}
	}
}

// TestFanOutRPC_PerAgentError captures dispatcher errors per agent
// without aborting the whole fan-out — failure of one agent must
// not poison results for others.
func TestFanOutRPC_PerAgentError(t *testing.T) {
	agentIDs := []string{"good", "bad", "good2"}
	disp := func(_ context.Context, id string, _ *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
		if id == "bad" {
			return nil, errors.New("boom")
		}
		return &v2pb.RpcResponse{}, nil
	}

	got := FanOutRPC(context.Background(), agentIDs, &v2pb.RpcRequest{}, FanOutOptions{
		MaxConcurrency: 4,
	}, disp)

	if len(got) != 3 {
		t.Fatalf("results = %d, want 3", len(got))
	}
	if got[0].Err != nil || got[2].Err != nil {
		t.Errorf("good agents must have nil err: got=%+v", got)
	}
	if got[1].Err == nil || got[1].Err.Error() != "boom" {
		t.Errorf("bad agent err = %v, want 'boom'", got[1].Err)
	}
	// Resp must be nil when Err is set; symmetrically populated
	// when Err is nil.
	if got[1].Resp != nil {
		t.Errorf("bad agent Resp must be nil when Err is set; got %+v", got[1].Resp)
	}
}

// TestFanOutRPC_MaxConcurrencyHonoured asserts the helper never has
// more than MaxConcurrency dispatcher calls in flight at once, and
// that the cap is actually exercised (otherwise the test would
// pass for a serialised implementation too).
func TestFanOutRPC_MaxConcurrencyHonoured(t *testing.T) {
	const limit = 3
	agentIDs := make([]string, 12)
	for i := range agentIDs {
		agentIDs[i] = fmt.Sprintf("agent-%02d", i)
	}

	var inFlight atomic.Int64
	var maxSeen atomic.Int64
	// Barrier: dispatchers stack until close(release) lets them all
	// out. The test below blocks on `stacked` to confirm the
	// in-flight count actually reached `limit` before releasing.
	release := make(chan struct{})
	stacked := make(chan struct{}, 1)

	disp := func(_ context.Context, _ string, _ *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
		now := inFlight.Add(1)
		for {
			old := maxSeen.Load()
			if now <= old || maxSeen.CompareAndSwap(old, now) {
				break
			}
		}
		// Signal once we've reached `limit` — main goroutine then
		// closes the release channel.
		if now == int64(limit) {
			select {
			case stacked <- struct{}{}:
			default:
			}
		}
		<-release
		inFlight.Add(-1)
		return &v2pb.RpcResponse{}, nil
	}

	done := make(chan struct{})
	go func() {
		FanOutRPC(context.Background(), agentIDs, &v2pb.RpcRequest{}, FanOutOptions{
			MaxConcurrency: limit,
		}, disp)
		close(done)
	}()

	select {
	case <-stacked:
	case <-time.After(2 * time.Second):
		t.Fatalf("never reached limit=%d in flight (maxSeen=%d)",
			limit, maxSeen.Load())
	}
	close(release)
	<-done

	if got := maxSeen.Load(); got > int64(limit) {
		t.Errorf("max concurrent dispatcher calls = %d, want <= %d", got, limit)
	}
	if got := maxSeen.Load(); got < int64(limit) {
		t.Errorf("max concurrent dispatcher calls = %d, want exactly %d (limit not exercised)", got, limit)
	}
}

// TestFanOutRPC_ZeroConcurrencyDefaults ensures MaxConcurrency=0
// doesn't deadlock — the helper picks a sensible default rather
// than serialising or refusing.
func TestFanOutRPC_ZeroConcurrencyDefaults(t *testing.T) {
	agentIDs := []string{"a", "b", "c", "d"}
	disp := func(_ context.Context, _ string, _ *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
		return &v2pb.RpcResponse{}, nil
	}
	done := make(chan []FanOutResult, 1)
	go func() {
		done <- FanOutRPC(context.Background(), agentIDs, &v2pb.RpcRequest{}, FanOutOptions{
			MaxConcurrency: 0,
		}, disp)
	}()
	select {
	case got := <-done:
		if len(got) != len(agentIDs) {
			t.Errorf("len results = %d, want %d", len(got), len(agentIDs))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FanOutRPC deadlocked with MaxConcurrency=0")
	}
}

// TestFanOutRPC_ContextCancel stops dispatching mid-flight when the
// caller's context fires. Unstarted agents must surface ctx.Err()
// rather than silently being skipped.
func TestFanOutRPC_ContextCancel(t *testing.T) {
	agentIDs := []string{"a", "b", "c", "d", "e"}
	ctx, cancel := context.WithCancel(context.Background())

	var started atomic.Int64
	disp := func(c context.Context, _ string, _ *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
		started.Add(1)
		<-c.Done()
		return nil, c.Err()
	}

	doneCh := make(chan []FanOutResult, 1)
	go func() {
		doneCh <- FanOutRPC(ctx, agentIDs, &v2pb.RpcRequest{}, FanOutOptions{
			MaxConcurrency: 2,
		}, disp)
	}()

	// Wait for the first slot of dispatchers to be in flight, then
	// cancel.
	for started.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()

	got := <-doneCh
	if len(got) != len(agentIDs) {
		t.Fatalf("len results = %d, want %d", len(got), len(agentIDs))
	}
	// At least one agent must surface ctx.Canceled (the in-flight
	// ones for sure; pending ones should too).
	hasCanceled := false
	for _, r := range got {
		if errors.Is(r.Err, context.Canceled) {
			hasCanceled = true
		}
	}
	if !hasCanceled {
		// Print the result table for debugging.
		ids := make([]string, len(got))
		for i, r := range got {
			if r.Err != nil {
				ids[i] = fmt.Sprintf("%s=%v", r.AgentID, r.Err)
			} else {
				ids[i] = fmt.Sprintf("%s=ok", r.AgentID)
			}
		}
		sort.Strings(ids)
		t.Errorf("no ctx.Canceled errors in results: %v", ids)
	}
}

// TestFanOutRPC_EmptyInput: zero agents → zero results, no panic.
func TestFanOutRPC_EmptyInput(t *testing.T) {
	disp := func(_ context.Context, _ string, _ *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
		t.Fatalf("dispatcher called for empty input")
		return nil, nil
	}
	got := FanOutRPC(context.Background(), nil, &v2pb.RpcRequest{}, FanOutOptions{
		MaxConcurrency: 4,
	}, disp)
	if got != nil && len(got) != 0 {
		t.Errorf("results = %v, want empty", got)
	}
}
