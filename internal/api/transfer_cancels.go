package api

import (
	"context"
	"sync"
)

// TransferCancelRegistry maps transfer IDs to context cancel
// functions for in-flight transfers. The archive HTTP handler
// registers each transfer as it kicks off and removes it as the
// stream finishes. The cancel HTTP handler looks up the entry by
// ID and invokes its cancel function, which propagates through the
// in-flight goroutine and tears down the agent stream + HTTP
// response.
//
// Implementation notes:
//   - The registry is a process-local in-memory map. Cancellation
//     therefore only works when the cancel-issuing API request lands
//     on the same server process as the in-flight transfer. In a
//     load-balanced deployment we'd persist a "cancel requested" bit
//     in the DB and have the streaming goroutine poll it; that
//     refactor stays an option without breaking this API.
//   - Cancel returns false when the ID is unknown — useful so the
//     HTTP cancel handler can respond 404 cleanly rather than
//     pretending the cancel succeeded.
type TransferCancelRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// NewTransferCancelRegistry constructs an empty registry.
func NewTransferCancelRegistry() *TransferCancelRegistry {
	return &TransferCancelRegistry{cancels: map[string]context.CancelFunc{}}
}

// Register stores the cancel func for id. Overwrites any prior
// entry for the same id (transfers SHOULD have unique ids;
// overwriting is just defensive).
func (r *TransferCancelRegistry) Register(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[id] = cancel
}

// Unregister removes the entry for id, no-op when absent. The
// owner SHOULD call this from a defer on the streaming goroutine
// so we don't leak entries for completed transfers.
func (r *TransferCancelRegistry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
}

// Cancel triggers the cancel func for id, returning true when an
// entry was found. The entry is left in place; the streaming
// goroutine clears it via Unregister once it observes the cancel.
func (r *TransferCancelRegistry) Cancel(id string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[id]
	r.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// Active returns the count of in-flight transfers — handy for the
// /info endpoint or unit tests.
func (r *TransferCancelRegistry) Active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cancels)
}
