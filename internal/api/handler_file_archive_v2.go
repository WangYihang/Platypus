package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// progressFlushBytes is the soft target for bytes streamed before
// we update the file_transfers row + broadcast a WS event. Larger
// values reduce DB write amplification but coarser progress;
// 512 KiB is a comfortable middle for ~10 Hz progress updates on
// a 5 MB/s connection.
const progressFlushBytes = 512 * 1024

// progressFlushInterval bounds the wall-clock time between progress
// updates so a slow stream still ticks the UI forward.
const progressFlushInterval = 250 * time.Millisecond

// TransferRecorder is the persistence + broadcast surface the
// archive handler uses to record file_transfers rows. Production
// implementations write to SQLite + broadcast WS events; tests
// substitute an in-memory fake (FakeTransferRecorder).
type TransferRecorder interface {
	Create(ctx context.Context, ft *storage.FileTransfer) error
	UpdateProgress(ctx context.Context, id string, bytes, total int64) error
	Finish(ctx context.Context, id, status string, bytes int64, errMsg string, at time.Time) error
}

// FileArchiveDeps is the set of collaborators
// RegisterV2FileArchiveRoutes needs. Bundling them keeps the call
// site readable as we add more (cancellation registry, ID
// generator, broadcaster).
type FileArchiveDeps struct {
	Service     *core.AgentLinkService
	RBAC        *RBAC
	Recorder    TransferRecorder
	Broadcaster *EventBroadcaster // optional; nil to skip WS events
	Activity    *activity.Recorder // optional; nil to skip audit logging
	Cancels     *TransferCancelRegistry
	IDGenerator func() string // optional; defaults to uuid.NewString
}

// FileTransferEvent is the wire shape of WS payloads for transfer
// state changes. Mirrors the storage layer so the frontend can
// render rows without a separate API round-trip after each event.
type FileTransferEvent struct {
	ID               string  `json:"id"`
	ProjectID        string  `json:"project_id"`
	HostID           string  `json:"host_id"`
	UserID           string  `json:"user_id"`
	Direction        string  `json:"direction"`
	Kind             string  `json:"kind"`
	Format           string  `json:"format"`
	Paths            []string `json:"paths"`
	Status           string  `json:"status"`
	BytesTransferred int64   `json:"bytes_transferred"`
	TotalBytes       int64   `json:"total_bytes"`
	ErrorMessage     string  `json:"error_message,omitempty"`
	StartedAt        string  `json:"started_at"`
	FinishedAt       string  `json:"finished_at,omitempty"`
}

// EventTypeFileTransferUpdated is the WS event type for transfer
// progress + state changes. Frontend subscribers filter on this
// string.
const EventTypeFileTransferUpdated = "file_transfer_updated"

// archiveRequestBody is the JSON the frontend POSTs to /fs/archive.
// All fields optional but a meaningful request will set paths +
// format.
type archiveRequestBody struct {
	Paths            []string `json:"paths"`
	Format           string   `json:"format"`
	FollowSymlinks   bool     `json:"follow_symlinks"`
	CompressionLevel int32    `json:"compression_level"`
}

// scanRequestBody is the JSON for /fs/scan: just paths.
type scanRequestBody struct {
	Paths          []string `json:"paths"`
	FollowSymlinks bool     `json:"follow_symlinks"`
}

// RegisterV2FileArchiveRoutes mounts:
//   POST /fs/scan      — walk paths, return totals (used to size progress)
//   POST /fs/archive   — stream a tar/tar.gz/zip archive of paths
//
// scan is viewer-tier (read), archive is also viewer-tier (it's a
// download); cancellation lives under the operator-tier API for
// /transfers/:id.
func RegisterV2FileArchiveRoutes(engine *gin.Engine, deps FileArchiveDeps) {
	if deps.IDGenerator == nil {
		deps.IDGenerator = uuid.NewString
	}
	if deps.Cancels == nil {
		deps.Cancels = NewTransferCancelRegistry()
	}
	base := engine.Group("/api/v1/projects/:pid/agents/:agent_id")
	base.Use(deps.RBAC.RequireAuth())

	viewer := base.Group("")
	viewer.Use(
		deps.RBAC.RequireProjectRole("pid", user.RoleViewer),
		deps.RBAC.RequireAgentInProject("pid", "agent_id"),
	)
	viewer.POST("/fs/scan", v2FileScan(deps))
	viewer.POST("/fs/archive", v2FileArchive(deps))
}

// v2FileScan opens a STREAM_TYPE_FILE_SCAN stream and returns the
// FileScanResponse as JSON.
func v2FileScan(deps FileArchiveDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := lookupAgent(c, deps.Service)
		if !ok {
			return
		}
		var body scanRequestBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.String(http.StatusBadRequest, "invalid body: %s", err)
			return
		}
		if len(body.Paths) == 0 {
			c.String(http.StatusBadRequest, "paths required")
			return
		}
		resp, err := runScan(c.Request.Context(), sess, body)
		if err != nil {
			c.String(http.StatusBadGateway, "scan: %s", err)
			return
		}
		if resp.Error != "" {
			c.JSON(http.StatusOK, gin.H{
				"file_count":  resp.FileCount,
				"dir_count":   resp.DirCount,
				"total_bytes": resp.TotalBytes,
				"error":       resp.Error,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"file_count":  resp.FileCount,
			"dir_count":   resp.DirCount,
			"total_bytes": resp.TotalBytes,
		})
	}
}

func runScan(ctx context.Context, sess *link.Session, body scanRequestBody) (*v2pb.FileScanResponse, error) {
	meta, _ := proto.Marshal(&v2pb.FileScanRequest{
		Paths:          body.Paths,
		FollowSymlinks: body.FollowSymlinks,
	})
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_SCAN, meta, "fs-scan")
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()
	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(stream, &resp); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	_ = ctx // reserved for future deadline propagation
	return &resp, nil
}

// v2FileArchive streams a tar/tar.gz/zip archive of the requested
// paths. Wire shape:
//   1. Pre-scan (best-effort) so we can set X-Total-Bytes.
//   2. Create file_transfers row in status=running.
//   3. Open STREAM_TYPE_FILE_ARCHIVE on the agent.
//   4. Read FileArchiveResponse ack.
//   5. Pipe FileChunk frames to HTTP response, counting bytes,
//      flushing progress on size+time triggers.
//   6. On EOF / error / cancel: finalize the transfer row +
//      broadcast a final WS event.
func v2FileArchive(deps FileArchiveDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := lookupAgent(c, deps.Service)
		if !ok {
			return
		}
		var body archiveRequestBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.String(http.StatusBadRequest, "invalid body: %s", err)
			return
		}
		if len(body.Paths) == 0 {
			c.String(http.StatusBadRequest, "paths required")
			return
		}
		fmtEnum, ext, ctype, ok := resolveArchiveFormat(body.Format)
		if !ok {
			c.String(http.StatusBadRequest, "unsupported format: %q (want tar/tar.gz/zip)", body.Format)
			return
		}

		projectID := c.Param("pid")
		agentID := c.Param("agent_id")
		userIDStr := ""
		if claims, ok := ClaimsFromContext(c); ok && claims != nil {
			userIDStr = claims.UserID
		}

		// Best-effort pre-scan to size progress. We don't fail the
		// download on scan errors; progress just becomes
		// indeterminate (TotalBytes=0) when the scan can't run.
		var totalBytes int64
		if scan, err := runScan(c.Request.Context(), sess, scanRequestBody{
			Paths: body.Paths, FollowSymlinks: body.FollowSymlinks,
		}); err == nil && scan.Error == "" {
			totalBytes = scan.TotalBytes
		}

		transferID := deps.IDGenerator()
		pathsJSON, _ := json.Marshal(body.Paths)
		now := time.Now().UTC()
		ft := &storage.FileTransfer{
			ID:         transferID,
			ProjectID:  projectID,
			HostID:     agentID,
			UserID:     userIDStr,
			Direction:  storage.TransferDirectionDownload,
			Kind:       storage.TransferKindArchive,
			Format:     extWithoutDot(ext),
			PathsJSON:  string(pathsJSON),
			Status:     storage.TransferStatusRunning,
			TotalBytes: totalBytes,
			StartedAt:  now,
		}
		if err := deps.Recorder.Create(c.Request.Context(), ft); err != nil {
			log.Warn("file_transfer create: %v", err)
		}
		broadcastTransfer(deps, ft, body.Paths)

		// Per-transfer cancellable context so the cancel API can
		// abort the in-flight stream.
		streamCtx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		deps.Cancels.Register(transferID, cancel)
		defer deps.Cancels.Unregister(transferID)

		meta, _ := proto.Marshal(&v2pb.FileArchiveRequest{
			Paths:            body.Paths,
			Format:           fmtEnum,
			FollowSymlinks:   body.FollowSymlinks,
			CompressionLevel: body.CompressionLevel,
		})
		stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE, meta, "fs-arch-"+transferID)
		if err != nil {
			finalizeTransfer(deps, ft, storage.TransferStatusFailed, 0, err.Error(), body.Paths)
			c.String(http.StatusBadGateway, "open stream: %s", err)
			return
		}
		// Closing the stream when the streamCtx fires cuts the
		// agent off cleanly and lets us return promptly on cancel.
		// yamux.Stream.Close is half-close — it doesn't abort an
		// in-flight Read while the peer keeps sending. Setting a
		// past read deadline forces the read to error out
		// immediately with the deadline error so the main loop
		// notices the cancel.
		go func() {
			<-streamCtx.Done()
			if dl, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
				_ = dl.SetReadDeadline(time.Now().Add(-time.Second))
			}
			_ = stream.Close()
		}()

		var hdr v2pb.FileArchiveResponse
		if err := link.ReadFrame(stream, &hdr); err != nil {
			finalizeTransfer(deps, ft, statusFromCtx(streamCtx, storage.TransferStatusFailed),
				0, err.Error(), body.Paths)
			c.String(http.StatusBadGateway, "read header: %s", err)
			return
		}
		if hdr.Error != "" {
			finalizeTransfer(deps, ft, storage.TransferStatusFailed, 0, hdr.Error, body.Paths)
			c.String(http.StatusBadGateway, "agent: %s", hdr.Error)
			return
		}

		// Now we're committed: emit response headers and stream.
		c.Writer.Header().Set("Content-Type", ctype)
		c.Writer.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="archive%s"`, ext))
		c.Writer.Header().Set("X-Transfer-Id", transferID)
		if totalBytes > 0 {
			c.Writer.Header().Set("X-Total-Bytes", fmt.Sprintf("%d", totalBytes))
		}
		c.Status(http.StatusOK)
		c.Writer.Flush()

		// Stream chunks while ticking progress on size+time triggers.
		var bytesOut int64
		lastFlushBytes := int64(0)
		lastFlushAt := time.Now()
		for {
			var ch v2pb.FileChunk
			if err := link.ReadFrame(stream, &ch); err != nil {
				finalStatus := storage.TransferStatusFailed
				errMsg := ""
				if errors.Is(err, io.EOF) {
					// EOF without explicit eof=true frame is treated
					// as a clean close (some agent crashes look like
					// this). Mark done with whatever we got.
					finalStatus = storage.TransferStatusDone
				} else {
					errMsg = err.Error()
					finalStatus = statusFromCtx(streamCtx, storage.TransferStatusFailed)
				}
				finalizeTransfer(deps, ft, finalStatus, bytesOut, errMsg, body.Paths)
				return
			}
			if len(ch.Data) > 0 {
				if _, werr := c.Writer.Write(ch.Data); werr != nil {
					// Client disconnect mid-stream — treat as cancel.
					finalizeTransfer(deps, ft, storage.TransferStatusCanceled, bytesOut, werr.Error(), body.Paths)
					return
				}
				atomic.AddInt64(&bytesOut, int64(len(ch.Data)))
				now := time.Now()
				if bytesOut-lastFlushBytes >= progressFlushBytes ||
					now.Sub(lastFlushAt) >= progressFlushInterval {
					_ = deps.Recorder.UpdateProgress(c.Request.Context(), transferID, bytesOut, totalBytes)
					ft.BytesTransferred = bytesOut
					broadcastTransfer(deps, ft, body.Paths)
					lastFlushBytes = bytesOut
					lastFlushAt = now
					c.Writer.Flush()
				}
			}
			if ch.Eof {
				if ch.Error != "" {
					finalizeTransfer(deps, ft, storage.TransferStatusFailed, bytesOut, ch.Error, body.Paths)
					return
				}
				finalizeTransfer(deps, ft, storage.TransferStatusDone, bytesOut, "", body.Paths)
				c.Writer.Flush()
				return
			}
		}
	}
}

// statusFromCtx maps a stream context's error onto the closest
// matching transfer status: a deliberately-cancelled context yields
// TransferStatusCanceled; anything else (timeout, plain error)
// yields the supplied fallback (typically Failed).
func statusFromCtx(ctx context.Context, fallback string) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return storage.TransferStatusCanceled
	}
	return fallback
}

func finalizeTransfer(deps FileArchiveDeps, ft *storage.FileTransfer, status string, bytes int64, errMsg string, paths []string) {
	at := time.Now().UTC()
	ft.Status = status
	ft.BytesTransferred = bytes
	ft.ErrorMessage = errMsg
	ft.FinishedAt = &at
	if err := deps.Recorder.Finish(context.Background(), ft.ID, status, bytes, errMsg, at); err != nil {
		log.Warn("file_transfer finish: %v", err)
	}
	broadcastTransfer(deps, ft, paths)
	if deps.Activity != nil {
		outcome := storage.OutcomeSuccess
		if status == storage.TransferStatusFailed {
			outcome = storage.OutcomeError
		}
		deps.Activity.Record(activity.Input{
			ProjectID:   strPtr(ft.ProjectID),
			ActorType:   storage.ActorTypeUser,
			ActorUser:   ft.UserID,
			Category:    storage.CategoryFile,
			Action:      "download.archive",
			TargetType:  "agent",
			TargetID:    ft.HostID,
			TargetLabel: strings.Join(paths, ", "),
			Outcome:     outcome,
			Error:       errMsg,
			Meta: map[string]any{
				"transfer_id":       ft.ID,
				"format":            ft.Format,
				"bytes_transferred": bytes,
				"status":            status,
			},
			At: at,
		})
	}
}

func broadcastTransfer(deps FileArchiveDeps, ft *storage.FileTransfer, paths []string) {
	if deps.Broadcaster == nil {
		return
	}
	ev := FileTransferEvent{
		ID:               ft.ID,
		ProjectID:        ft.ProjectID,
		HostID:           ft.HostID,
		UserID:           ft.UserID,
		Direction:        ft.Direction,
		Kind:             ft.Kind,
		Format:           ft.Format,
		Paths:            paths,
		Status:           ft.Status,
		BytesTransferred: ft.BytesTransferred,
		TotalBytes:       ft.TotalBytes,
		ErrorMessage:     ft.ErrorMessage,
		StartedAt:        ft.StartedAt.Format(time.RFC3339Nano),
	}
	if ft.FinishedAt != nil {
		ev.FinishedAt = ft.FinishedAt.Format(time.RFC3339Nano)
	}
	deps.Broadcaster.Broadcast(EventTypeFileTransferUpdated, ev)
}

// resolveArchiveFormat parses the JSON-supplied format string into
// the proto enum + the file extension and Content-Type to use for
// the response. Returns ok=false on unknown values.
func resolveArchiveFormat(s string) (v2pb.ArchiveFormat, string, string, bool) {
	switch strings.ToLower(s) {
	case "tar":
		return v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR, ".tar", "application/x-tar", true
	case "tar.gz", "tgz", "":
		return v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ, ".tar.gz", "application/gzip", true
	case "zip":
		return v2pb.ArchiveFormat_ARCHIVE_FORMAT_ZIP, ".zip", "application/zip", true
	}
	return v2pb.ArchiveFormat_ARCHIVE_FORMAT_UNSPECIFIED, "", "", false
}

func extWithoutDot(s string) string {
	return strings.TrimPrefix(s, ".")
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
