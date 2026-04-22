package api

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/storage"
)

// ActivityInput is the gin-aware twin of activity.Input. The fields map
// 1:1; we re-declare it here so handlers can stay inside the api
// package's namespace.
type ActivityInput = activity.Input

// NewActivityRecorder is re-exported for main.go.
func NewActivityRecorder(db *storage.DB) *activity.Recorder {
	return activity.New(db)
}

// SetActivityRecorder installs the process-wide recorder. Delegates
// to the underlying activity package so any call site — gin handler,
// core TCP handshake, enrollment service — resolves to the same
// recorder.
func SetActivityRecorder(r *activity.Recorder) {
	activity.SetRecorder(r)
}

// RecordActivity writes one audit row. If the request is authenticated,
// the actor id / role come from the JWT claims; network metadata
// (ClientIP, User-Agent, request id) and the project id (from :pid)
// are auto-filled so handlers only state what is action-specific.
func RecordActivity(c *gin.Context, in ActivityInput) {
	fillFromContext(c, &in)
	activity.Record(in)
}

// TimeActivity runs fn, measures its duration, and writes an activity
// row based on fn's return value. Returns fn's error unchanged.
func TimeActivity(c *gin.Context, in ActivityInput, fn func() error) error {
	fillFromContext(c, &in)
	return activity.Time(in, fn)
}

// RecordSystemActivity writes a row that is not tied to a gin request
// (server lifecycle, agent-side TCP handshake, background jobs). The
// caller is responsible for setting ActorType / ActorUser appropriately;
// the default is "system".
func RecordSystemActivity(ctx context.Context, in ActivityInput) {
	activity.RecordWithContext(ctx, in)
}

// fillFromContext populates ActorIP, ActorUA, RequestID, ProjectID,
// ActorUser, and ActorType from the gin context when they are left zero
// on the input. Treating each slot as "fill only if empty" lets callers
// override any field explicitly.
func fillFromContext(c *gin.Context, in *ActivityInput) {
	if c == nil {
		return
	}
	if in.ActorIP == "" {
		in.ActorIP = c.ClientIP()
	}
	if in.ActorUA == "" {
		in.ActorUA = c.Request.UserAgent()
	}
	if in.RequestID == "" {
		in.RequestID = c.GetString("request_id")
	}
	if in.ProjectID == nil {
		if pid := c.Param("pid"); pid != "" {
			pidCopy := pid
			in.ProjectID = &pidCopy
		}
	}
	if in.ActorUser == "" {
		if claims, ok := ClaimsFromContext(c); ok {
			in.ActorUser = claims.UserID
		}
	}
	if in.ActorType == "" {
		if in.ActorUser != "" {
			in.ActorType = storage.ActorTypeUser
		} else {
			in.ActorType = storage.ActorTypeAnonymous
		}
	}
}
