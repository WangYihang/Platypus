package storage

import "errors"

// ErrNotFound is returned by repo methods when a single-row lookup yields no
// rows. Handlers map it to a 404; other errors propagate as 500. Keeping it
// as a sentinel (not a wrapping type) lets callers use == for simplicity.
var ErrNotFound = errors.New("storage: not found")
