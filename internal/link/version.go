package link

// ProtocolVersion is the wire-protocol version number this binary
// speaks. It is sent on every fresh EnrollRequest (and on each
// SysInfoResponse refresh) so the server can record per-host
// compatibility metadata and gate features when needed.
//
// Bumping rules (deliberately strict — this is the one knob we
// promise to keep monotonic across releases):
//
//   - Adding optional fields to existing messages: NO bump. proto3
//     tolerates unknown fields and the absent-value reads back as
//     the zero value, so older peers keep working.
//   - Adding a new StreamType: NO bump. Older peers reject unknown
//     stream types with StreamReject{code="unsupported_type"}; the
//     opener handles the reject explicitly.
//   - Adding a new RpcRequest oneof case: NO bump. Same story —
//     older peers see the oneof as "not set" and reject.
//   - Removing a field, renaming a field, repurposing a field
//     number, changing framing, changing the meaning of an existing
//     enum value, or making a previously-optional field required:
//     YES, bump.
//
// MinSupportedProtocolVersion is the lowest version this binary is
// willing to talk to. Server-side, agents below this number are
// flagged in the host list (and, in a later pass, automatically
// queued for an upgrade push). Today both consts equal 1; the
// floor only moves forward when a real breaking change ships.
const (
	ProtocolVersion             uint32 = 1
	MinSupportedProtocolVersion uint32 = 1
)
