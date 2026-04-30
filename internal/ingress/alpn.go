// Package ingress holds the TLS bootstrap helpers for the platypus-
// server's HTTPS surface. The historical custom ALPN multiplexer
// (ptps-agent / ptps-mesh / h2 / http/1.1 demultiplexed at the TLS
// layer) is gone — agent links and mesh links both ride
// /api/v1/agent/link and /api/v1/mesh/link on the standard HTTP
// router, mTLS-authenticated end-to-end. What remains here is just
// the cert-source plumbing.
package ingress

// DefaultProtocols is the canonical NextProtos slice. Standard
// HTTP ALPN values only — h2 first because the server registers
// http2.ConfigureServer.
var DefaultProtocols = []string{"h2", "http/1.1"}
