package agent

// SigningPublicKey is the base64-encoded Ed25519 public key the agent
// uses to verify the distributor's update manifest signature before
// running a self-upgrade. The matching private key never leaves the
// release pipeline; agents that receive a STREAM_TYPE_AGENT_UPGRADE
// command refuse to install any binary whose manifest signature
// doesn't validate against this key.
//
// The value is injected at build time via -ldflags, see the
// AGENT_SIGNING_PUBKEY variable in the Makefile and the corresponding
// entry in .goreleaser.yaml. Local dev / make-snapshot builds leave
// it empty, in which case the agent simply refuses to self-upgrade —
// an unsigned channel is worse than no channel.
//
// Encoding is raw base64 (no PEM, no length prefix). Callers should
// base64-decode and feed the resulting 32 bytes to ed25519.Verify.
//
// Key rotation is intentionally NOT supported in this version: a
// fleet-wide rotation requires (1) cutting a release whose binary
// embeds the new pubkey, (2) signing that release with the old
// private key, (3) pushing it through self-upgrade, and only then
// (4) starting to sign with the new private key. Future revisions
// may move to a list and accept any-of, at which point this var
// becomes a comma-separated list — callers should treat splits on
// "," as forward-compatible.
var SigningPublicKey string
