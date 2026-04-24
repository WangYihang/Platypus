package api

// The types in this file exist mainly to give swaggo concrete schemas
// to generate from. They mirror the gin.H{...} objects that the
// handlers emit at runtime. Without these, @Success annotations of
// `map[string]any` collapse to useless `object` schemas in the
// OpenAPI spec.

// wsTicketResponse is the shape of POST /api/v1/ws/ticket.
type wsTicketResponse struct {
	Ticket string `json:"ticket"`
}
