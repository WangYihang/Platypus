// Package settings is the runtime-tunable policy layer for the server.
//
// The server carries two kinds of config today:
//   - Bootstrap config (YAML/env): infrastructure that the process needs
//     to start at all — bind addresses, TLS certs, DB path, secrets.
//     Lives in internal/utils/config and is read once on startup.
//   - Policy config (this package): admin-tunable knobs that operators
//     legitimately want to change without editing YAML + restarting the
//     server. Token TTLs, release channel, mesh discovery cadence.
//
// Policy reads go through Registry. Resolution order is:
//  1. in-process cache                      (hot path)
//  2. admin_settings DB row (UI-written)    (admin override)
//  3. YAML field on *config.Config          (operator default)
//  4. hardcoded default constants here      (safety net)
//
// Writes go through Set/Reset, which invalidate the cache entry for the
// touched key. The next read picks up the new value, so changes take
// effect immediately — no restart, no goroutine signalling, no polling.
// Every Set/Reset is audited via internal/activity with CategoryAdmin.
package settings

import "time"

// Registered Phase-1 keys. Adding a new one requires: a constant below,
// a typed accessor on Registry, a default constant, and a descriptor
// row in allDescriptors.
const (
	KeyAuthAccessTokenTTL       = "auth.access_token_ttl"
	KeyAuthRefreshTokenTTL      = "auth.refresh_token_ttl"
	KeyAuthPATDefaultTTL        = "auth.pat_default_ttl"
	KeyDistributorChannel       = "distributor.channel"
	KeyDistributorPresignedTTL  = "distributor.presigned_ttl"
	KeyMeshDiscoveryLAN         = "mesh.discovery_lan"
	KeyMeshDiscoveryIntervalSec = "mesh.discovery_interval_seconds"
)

// Hardcoded defaults — the last line of the resolution chain. Must
// match (or be sensible replacements for) the historical YAML
// defaults so behaviour is unchanged when no override is present.
const (
	DefaultAccessTokenTTL       = 15 * time.Minute
	DefaultRefreshTokenTTL      = 14 * 24 * time.Hour
	DefaultPATDefaultTTL        = time.Hour
	DefaultDistributorChannel   = "stable"
	DefaultPresignedTTL         = 5 * time.Minute
	DefaultMeshDiscoveryLAN     = true
	DefaultMeshDiscoveryIntSecs = 30
)

// valueType tags determine how the registry and HTTP layer serialise
// / validate a setting. Matches the descriptor Type field surfaced
// to the Web UI.
const (
	typeDurationSeconds = "duration_seconds"
	typeBool            = "bool"
	typeString          = "string"
	typeInt             = "int"
)
