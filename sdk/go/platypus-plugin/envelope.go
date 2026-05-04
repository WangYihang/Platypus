package platypus

import (
	"encoding/json"
)

// Envelope is the wire shape every Platypus host fn returns. Mirrors
// the Go-side `envelope` struct in
// internal/agent/plugin/host_funcs.go: success populates Data,
// failure populates Error with Ok=false. Plugin authors should always
// check Ok before using Data.
type Envelope struct {
	Ok    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Decode parses the JSON-marshalled envelope returned by a host fn.
// Returns the typed Data on success or a Go error on Ok=false.
func decodeEnvelope(raw []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

// Capability identifiers — informational; the agent enforces the
// real allowlist via the plugin manifest's capabilities map at
// install time. Plugin authors pick from this list when composing
// their plugin.yaml.
const (
	CapLog     = "log"
	CapKV      = "kv"
	CapFSRead  = "fs.read"
	CapFSWrite = "fs.write"
	CapExec    = "exec"
	CapHTTP    = "net.http"
	CapNetDial = "net.dial"
	CapProcess = "process"
	CapSysInfo = "sysinfo"
)
