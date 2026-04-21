package api

import "github.com/WangYihang/Platypus/internal/core"

// The types in this file exist mainly to give swaggo concrete schemas to
// generate from. They mirror the gin.H{...} objects that the handlers emit
// at runtime. Without these, @Success annotations of `map[string]any`
// collapse to useless `object` schemas in the OpenAPI spec.

// legacyAck is the {status:true, msg:"..."} envelope returned by the legacy
// endpoints on successful acknowledgement (no payload body).
type legacyAck struct {
	Status bool   `json:"status" example:"true"`
	Msg    string `json:"msg,omitempty"`
}

// legacyError is the {status:false, msg:"..."} envelope the legacy endpoints
// emit alongside proper 4xx/5xx codes. The status field is historically
// required by clients even though the HTTP status now carries the same signal.
type legacyError struct {
	Status bool   `json:"status" example:"false"`
	Msg    string `json:"msg"`
}

// legacyServer wraps a single listener in the legacy {status,msg} envelope.
type legacyServer struct {
	Status bool            `json:"status" example:"true"`
	Msg    *core.TCPServer `json:"msg"`
}

// legacyServerList wraps the listener + distributor snapshot returned by
// GET /api/server.
type legacyServerList struct {
	Status bool                   `json:"status" example:"true"`
	Msg    serversWithDistributor `json:"msg"`
}

// legacyClientMap wraps a hash→session map. Entries are heterogeneous
// (TCPClient or TermiteClient) so the value type is left as `object` in the
// generated spec; callers typically probe the `version` field to tell them
// apart.
type legacyClientMap struct {
	Status bool                   `json:"status" example:"true"`
	Msg    map[string]interface{} `json:"msg"`
}

// legacyClientEntry wraps a single session in the legacy envelope. The msg
// field is polymorphic (TCPClient or TermiteClient) and surfaced as a plain
// object in the OpenAPI spec.
type legacyClientEntry struct {
	Status bool        `json:"status" example:"true"`
	Msg    interface{} `json:"msg"`
}

// sizeResponse is the shape of GET /api/v1/sessions/{id}/files/size.
type sizeResponse struct {
	Status bool  `json:"status" example:"true"`
	Size   int64 `json:"size"`
}

// bytesWrittenResponse is the shape of POST /api/v1/sessions/{id}/files.
type bytesWrittenResponse struct {
	Status       bool `json:"status" example:"true"`
	BytesWritten int  `json:"bytes_written"`
}

// ackResponse is the shape of endpoints that only need to acknowledge
// (CreateTunnel, PatchSession).
type ackResponse struct {
	Status bool   `json:"status" example:"true"`
	Msg    string `json:"msg"`
}

// tunnelsResponse is the shape of GET /api/v1/sessions/{id}/tunnels.
type tunnelsResponse struct {
	Status  bool              `json:"status" example:"true"`
	Tunnels []tunnelInfoEntry `json:"tunnels"`
}

// wsTicketResponse is the shape of POST /api/v1/ws/ticket.
type wsTicketResponse struct {
	Ticket string `json:"ticket"`
}
