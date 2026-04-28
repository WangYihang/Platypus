package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/internal/log"
)

// Event represents a server-sent event via WebSocket.
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// EventBroadcaster wraps a melody instance for broadcasting events.
type EventBroadcaster struct {
	ws *melody.Melody
}

// NewEventBroadcaster creates a new event broadcaster with its own
// melody instance. Mostly useful in tests; production code should
// share the existing /notify channel via NewEventBroadcasterFromMelody
// so frontend subscribers see all event types in one connection.
func NewEventBroadcaster() *EventBroadcaster {
	ws := melody.New()
	ws.HandleConnect(func(s *melody.Session) {
		log.Info("Event client connected from: %s", s.Request.RemoteAddr)
	})
	ws.HandleDisconnect(func(s *melody.Session) {
		log.Info("Event client disconnected from: %s", s.Request.RemoteAddr)
	})
	return &EventBroadcaster{ws: ws}
}

// NewEventBroadcasterFromMelody wraps an existing melody (typically
// the /notify fan-out channel registered by RegisterWebSocketRoutes)
// so file-transfer events ride the same WS connection browsers
// already keep open for session lifecycle events.
func NewEventBroadcasterFromMelody(ws *melody.Melody) *EventBroadcaster {
	return &EventBroadcaster{ws: ws}
}

// Broadcast sends an event to all connected WebSocket clients.
func (eb *EventBroadcaster) Broadcast(eventType string, data interface{}) {
	event := Event{Type: eventType, Data: data}
	msg, err := json.Marshal(event)
	if err != nil {
		log.Error("Failed to marshal event: %s", err)
		return
	}
	eb.ws.Broadcast(msg)
}

// Handler returns a gin handler for WebSocket upgrade.
func (eb *EventBroadcaster) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		eb.ws.HandleRequest(c.Writer, c.Request)
	}
}
