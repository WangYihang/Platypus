package platypus

// Envelope is the wire shape every Platypus host fn returns. Mirrors
// the Go-side `envelope` struct in
// internal/agent/plugin/host_funcs.go: success populates Data,
// failure populates Error with Ok=false. Plugin authors should
// always check Ok before using Data.
//
// Data is raw JSON bytes (whatever the host fn put in the `data`
// field — could be a string, object, array, or null). Decode it
// with the standard library's encoding/json or your own parser as
// needed; the SDK no longer routes through json.Unmarshal here
// because TinyGo's reflect/json gap on map types makes that path
// unsafe even for map-free callers (any reachable map[K]V in the
// program triggers Type.Implements panics during the json type
// cache initialisation).
type Envelope struct {
	Ok    bool
	Data  []byte
	Error string
}

// decodeEnvelope hand-parses a top-level JSON object of the shape
// {"ok":bool, "data":<value>, "error":"<string>"}.  Order-tolerant.
// Returns an error only on malformed input — a well-formed envelope
// where ok=false sets Envelope.Error and Envelope.Ok=false; the
// caller decides what to do.
//
// Hand-rolled because TinyGo's encoding/json initialises a global
// type cache on first call that walks every reachable struct's
// fields and asks `Type.Implements(jsonMarshaler)` — TinyGo's
// reflect implementation panics on that question for any type whose
// methods touch interface assertions, which includes
// json.RawMessage.  Avoiding json.Unmarshal in the SDK keeps that
// cache off the hot path and lets plugins use json.Unmarshal /
// json.Marshal freely on their own map-free types.
func decodeEnvelope(raw []byte) (Envelope, error) {
	p := jsonParser{buf: raw}
	p.skipWhitespace()
	if !p.consume('{') {
		return Envelope{}, parseErr("expect '{'", p.pos)
	}
	var env Envelope
	for {
		p.skipWhitespace()
		if p.consume('}') {
			return env, nil
		}
		key, err := p.readString()
		if err != nil {
			return env, err
		}
		p.skipWhitespace()
		if !p.consume(':') {
			return env, parseErr("expect ':' after key", p.pos)
		}
		p.skipWhitespace()
		switch key {
		case "ok":
			b, err := p.readBool()
			if err != nil {
				return env, err
			}
			env.Ok = b
		case "data":
			start := p.pos
			if err := p.skipValue(); err != nil {
				return env, err
			}
			env.Data = p.buf[start:p.pos]
		case "error":
			s, err := p.readString()
			if err != nil {
				return env, err
			}
			env.Error = s
		default:
			// Skip unknown fields so future host-fn additions
			// don't break old plugins.
			if err := p.skipValue(); err != nil {
				return env, err
			}
		}
		p.skipWhitespace()
		if p.consume(',') {
			continue
		}
		p.skipWhitespace()
		if !p.consume('}') {
			return env, parseErr("expect ',' or '}'", p.pos)
		}
		return env, nil
	}
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
