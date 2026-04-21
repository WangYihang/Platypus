package api

// The types in this file exist mainly to give swaggo concrete schemas to
// generate from. They mirror the gin.H{...} objects that the handlers emit
// at runtime. Without these, @Success annotations of `map[string]any`
// collapse to useless `object` schemas in the OpenAPI spec.

// sessionEntry wraps a single session in the {status, msg} envelope used by
// PATCH, GET, and POST /sessions/:id/gather. Msg is the raw AgentClient.
type sessionEntry struct {
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
