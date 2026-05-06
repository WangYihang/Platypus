package core

import (
	"context"
	"sync"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// FanOutDispatcher is the per-agent RPC entry point FanOutRPC calls
// in parallel. Production callers pass a closure over CallAgentRPC;
// tests inject a stub. Returning a nil response is allowed iff err
// is non-nil.
type FanOutDispatcher func(ctx context.Context, agentID string, req *v2pb.RpcRequest) (*v2pb.RpcResponse, error)

// FanOutOptions tune the parallel dispatch. Zero values pick sane
// defaults so callers can leave the struct blank for the common
// case.
type FanOutOptions struct {
	// MaxConcurrency caps the number of dispatcher calls in flight
	// at once. <= 0 falls back to defaultFanOutConcurrency. Capped
	// at len(agentIDs) so a 2-agent fan-out doesn't allocate a
	// 64-slot semaphore.
	MaxConcurrency int
}

// defaultFanOutConcurrency balances latency against link saturation.
// 32 is conservative — most fleets run in the low hundreds; larger
// installs should tune via FanOutOptions.
const defaultFanOutConcurrency = 32

// FanOutResult is the per-agent outcome. Exactly one of Resp/Err is
// populated. AgentID echoes the input slice's value at the same
// index so callers can correlate via either index OR id (the
// fan-out preserves input order).
type FanOutResult struct {
	AgentID string
	Resp    *v2pb.RpcResponse
	Err     error
}

// FanOutRPC dispatches the same RpcRequest to N agents in parallel,
// gating concurrency to opts.MaxConcurrency. The returned slice has
// one entry per input agent in input order; pending agents whose
// slot never opened (because the parent ctx fired) surface the
// ctx error so callers don't see silent gaps.
//
// Each goroutine's dispatcher inherits the parent ctx; cancellation
// propagates to in-flight dispatcher calls naturally.
func FanOutRPC(
	ctx context.Context,
	agentIDs []string,
	req *v2pb.RpcRequest,
	opts FanOutOptions,
	disp FanOutDispatcher,
) []FanOutResult {
	if len(agentIDs) == 0 {
		return nil
	}
	limit := opts.MaxConcurrency
	if limit <= 0 {
		limit = defaultFanOutConcurrency
	}
	if limit > len(agentIDs) {
		limit = len(agentIDs)
	}

	results := make([]FanOutResult, len(agentIDs))
	for i, id := range agentIDs {
		results[i].AgentID = id
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, id := range agentIDs {
		i, id := i, id
		// Pre-flight cancellation check: if the parent ctx is
		// already canceled, mark unstarted agents with ctx.Err()
		// rather than queuing a goroutine that does the same.
		select {
		case <-ctx.Done():
			results[i].Err = ctx.Err()
			continue
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			resp, err := disp(ctx, id, req)
			if err != nil {
				results[i].Err = err
				return
			}
			results[i].Resp = resp
		}()
	}
	wg.Wait()
	return results
}
