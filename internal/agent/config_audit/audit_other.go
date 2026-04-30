//go:build !linux

package config_audit

// On non-Linux builds we register no auditors. Audit() still returns a
// well-formed (empty) result and the RPC handler stays available, so
// the UI doesn't need a "scanning supported?" probe — it simply finds
// zero auditors and renders an empty checklist.
