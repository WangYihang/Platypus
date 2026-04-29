//go:build !linux

package security

// On non-Linux builds the security package compiles with no
// registered checkers. The scan still runs and returns an empty
// finding list — the agent advertises the RPC handler so callers
// don't need a separate "is scanning supported" probe.
