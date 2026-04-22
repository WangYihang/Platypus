package core

import (
	"encoding/json"

	"github.com/WangYihang/Platypus/internal/log"
)

// Notify event constants. Dotted-lowercase form mirrors the scheme used
// for protobuf message types and keeps client-side subscribe calls
// readable (onNotify("session.opened", ...)). Pinning them as package
// constants is what guards the wire format — the test in notify_test.go
// asserts the exact strings.
const (
	EventHostSeen        = "host.seen"
	EventSessionOpened   = "session.opened"
	EventSessionClosed   = "session.closed"
	EventListenerCreated = "listener.created"
	EventListenerDeleted = "listener.deleted"

	// Topology events. The 1 Hz coalescer in core/topology_stream.go
	// batches rapid stat changes into one link_stats / machine_stats
	// frame per second; link_up / link_down fire immediately on the
	// observed edge.
	EventTopologyLinkUp       = "topology.link_up"
	EventTopologyLinkDown     = "topology.link_down"
	EventTopologyLinkStats    = "topology.link_stats"
	EventTopologyMachineStats = "topology.machine_stats"
	EventTopologyNodeJoined   = "topology.node_joined"
	EventTopologyNodeLeft     = "topology.node_left"
)

// notifyEnvelope is the JSON wrapper every /notify message is shipped in.
// Frontend dispatches on Type and only unmarshals Data when it has a
// subscriber for that type.
type notifyEnvelope struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// BuildNotifyMessage serialises an event + payload to the wire envelope.
// Exposed so tests can assert the shape without spinning up a melody
// server.
func BuildNotifyMessage(eventType string, data interface{}) ([]byte, error) {
	return json.Marshal(notifyEnvelope{Type: eventType, Data: data})
}

// BroadcastNotify sends the given event to every connected /notify
// client. A nil NotifyWebSocket (subsystem disabled, or server started
// without REST) makes this a no-op — emitters don't have to guard.
// Serialisation errors are logged rather than propagated; they'd only
// indicate an upstream bug in the payload shape.
func BroadcastNotify(eventType string, data interface{}) {
	if Ctx == nil || Ctx.NotifyWebSocket == nil {
		return
	}
	msg, err := BuildNotifyMessage(eventType, data)
	if err != nil {
		log.Error("BroadcastNotify serialise %s: %s", eventType, err)
		return
	}
	if err := Ctx.NotifyWebSocket.Broadcast(msg); err != nil {
		// Broadcast only errors if all sessions are closed — worth a
		// debug log, not an alarm.
		log.Info("BroadcastNotify %s: %s", eventType, err)
	}
}
