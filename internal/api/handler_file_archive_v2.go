package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
//
// `bytes` is the *source* progress (uncompressed bytes processed so
// far, comparable to the pre-scan total). `wireBytes` is the
// *post-encoding* count — the bytes actually written to the HTTP
// response body, used for compression-ratio + network-speed display.
// For non-archive transfers the two are equal.
type TransferRecorder interface {
	Create(ctx context.Context, ft *storage.FileTransfer) error
	UpdateProgress(ctx context.Context, id string, bytes, wireBytes, total int64) error
	Finish(ctx context.Context, id, status string, bytes, wireBytes int64, errMsg string, at time.Time) error
}

// HostLookup is the slim subset of *storage.HostRepo that the file
// transfer handlers need: just the agent_id → host translation. We
// take an interface (instead of the full repo) so tests can stub
// with a 5-line fake; production wires `db.Hosts()`.
type HostLookup interface {
	GetByAgentID(ctx context.Context, agentID string) (*storage.Host, error)
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
	// Hosts resolves an agent_id to its host UUID so file_transfers
	// rows hold the real host_id (matching the column's name) instead
	// of the agent id. Optional: when nil, transfers fall back to
	// storing the agent_id (legacy behaviour) so tests that don't
	// care about host scoping aren't forced to seed a host repo.
	Hosts HostLookup
	// PreviewSigner mints + verifies the short-lived URL tokens that
	// browser <video>/<audio>/pdf.js elements use to authenticate
	// /fs/read without an Authorization header. Optional in tests
	// that don't exercise the preview-token route, but production
	// must wire one or the browser-direct preview path stays disabled.
	PreviewSigner *PreviewSigner
}

// FileTransferEvent is the wire shape of WS payloads for transfer
// state changes. Mirrors the storage layer so the frontend can
// render rows without a separate API round-trip after each event.
type FileTransferEvent struct {
	ID               string   `json:"id"`
	ProjectID        string   `json:"project_id"`
	HostID           string   `json:"host_id"`
	UserID           string   `json:"user_id"`
	Direction        string   `json:"direction"`
	Kind             string   `json:"kind"`
	Format           string   `json:"format"`
	Paths            []string `json:"paths"`
	Status           string   `json:"status"`
	BytesTransferred int64    `json:"bytes_transferred"`
	WireBytes        int64    `json:"wire_bytes"`
	TotalBytes       int64    `json:"total_bytes"`
	ErrorMessage     string   `json:"error_message,omitempty"`
	StartedAt        string   `json:"started_at"`
	FinishedAt       string   `json:"finished_at,omitempty"`
}

// EventTypeFileTransferUpdated is the WS event type for transfer
// progress + state changes. Frontend subscribers filter on this
// string.
const EventTypeFileTransferUpdated = "file_transfer_updated"

// previewTokenRequestBody is the JSON shape POSTed to
// /fs/preview-token. Only `path` is required; the (pid, agent_id)
// pair come from the URL.
type previewTokenRequestBody struct {
	Path string `json:"path"`
}

// previewTokenResponse is the wire shape returned to the frontend so
// it can drop the result straight into a <video src=...>. The URL is
// the canonical /fs/read path with all three signed query params
// (path, exp, preview_token) already filled in — the caller doesn't
// need to know the signing format.
type previewTokenResponse struct {
	Token string `json:"token"`
	Exp   int64  `json:"exp"`
	URL   string `json:"url"`
}

// v2FilePreviewTokenMint signs a short-lived URL token authorising
// browser-direct reads of (project, agent, path). Bearer-auth gated;
// the gate is what binds the token to a real user (the token itself
// carries no user identity, only the resource).
func v2FilePreviewTokenMint(deps FileArchiveDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.PreviewSigner == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				gin.H{"error": "preview signer not configured"})
			return
		}
		var body previewTokenRequestBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.String(http.StatusBadRequest, "invalid body: %s", err)
			return
		}
		if body.Path == "" {
			c.String(http.StatusBadRequest, "path required")
			return
		}
		pid := c.Param("pid")
		aid := c.Param("agent_id")
		token, exp := deps.PreviewSigner.Sign(pid, aid, body.Path)

		// Build the full URL the caller can use directly. Path goes
		// through QueryEscape because filesystem paths legitimately
		// contain %, &, # and other reserved chars.
		q := url.Values{}
		q.Set("path", body.Path)
		q.Set("exp", strconv.FormatInt(exp, 10))
		q.Set("preview_token", token)
		fullURL := "/api/v1/projects/" + pid + "/agents/" + aid + "/fs/read?" + q.Encode()

		c.JSON(http.StatusOK, previewTokenResponse{
			Token: token,
			Exp:   exp,
			URL:   fullURL,
		})
	}
}

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

	// /fs/read is split off because of its dual auth path: the
	// browser's <video src> / <audio src> / pdf.js URL fetch can't
	// set an Authorization header, so the route must additionally
	// accept a short-lived signed-URL token. RequireFsReadAuth
	// branches on the presence of ?preview_token= and runs either
	// the bearer chain (Bearer + project role + agent-in-project)
	// or the token-verification chain inline. The other routes
	// can't be reached from a browser-native element so they stay
	// on the standard chain.
	fsRead := base.Group("")
	fsRead.Use(deps.RBAC.RequireFsReadAuth(deps.PreviewSigner))
	fsRead.GET("/fs/read", v2FileDownloadTracked(deps))

	authed := base.Group("")
	authed.Use(deps.RBAC.RequireAuth())

	viewer := authed.Group("")
	viewer.Use(
		deps.RBAC.RequireProjectRole("pid", user.RoleViewer),
		deps.RBAC.RequireAgentInProject("pid", "agent_id"),
	)
	viewer.POST("/fs/scan", v2FileScan(deps))
	viewer.POST("/fs/archive", v2FileArchive(deps))
	// /fs/preview-token mints the signed URL the frontend then drops
	// into <video src=...>. Bearer-only — anonymous mints would defeat
	// the point of the token in the first place.
	viewer.POST("/fs/preview-token", v2FilePreviewTokenMint(deps))

	// /fs/upload is operator-tier — it MUTATES the agent's filesystem.
	// Same wire shape as /fs/write but threads a file_transfers row,
	// progress ticks, audit log, and cancel-registry entry through the
	// same plumbing the /fs/archive download path uses.
	operator := authed.Group("")
	operator.Use(
		deps.RBAC.RequireProjectRole("pid", user.RoleOperator),
		deps.RBAC.RequireAgentInProject("pid", "agent_id"),
	)
	operator.PUT("/fs/upload", v2FileUploadTracked(deps))
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

		// Pre-scan to size the progress bar. The agent walks the
		// requested roots once and reports their uncompressed
		// content size; FileChunk.source_bytes_so_far is in the
		// same units, so progress = source/total is a real
		// percentage even for tar.gz / zip downloads. A scan
		// failure is non-fatal — the transfer just degrades to
		// indeterminate progress until the chunks start arriving.
		var totalBytes int64
		if scanResp, err := runScan(c.Request.Context(), sess, scanRequestBody{
			Paths: body.Paths, FollowSymlinks: body.FollowSymlinks,
		}); err == nil && scanResp != nil && scanResp.Error == "" {
			totalBytes = scanResp.TotalBytes
		}

		transferID := deps.IDGenerator()
		pathsJSON, _ := json.Marshal(body.Paths)
		now := time.Now().UTC()
		ft := &storage.FileTransfer{
			ID:         transferID,
			ProjectID:  projectID,
			HostID:     resolveHostID(c.Request.Context(), deps, agentID),
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
			finalizeTransfer(deps, ft, storage.TransferStatusFailed, 0, 0, err.Error(), body.Paths)
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
				0, 0, err.Error(), body.Paths)
			c.String(http.StatusBadGateway, "read header: %s", err)
			return
		}
		if hdr.Error != "" {
			finalizeTransfer(deps, ft, storage.TransferStatusFailed, 0, 0, hdr.Error, body.Paths)
			c.String(http.StatusBadGateway, "agent: %s", hdr.Error)
			return
		}

		// Now we're committed: emit response headers and stream.
		c.Writer.Header().Set("Content-Type", ctype)
		c.Writer.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="archive%s"`, ext))
		c.Writer.Header().Set("X-Transfer-Id", transferID)
		if totalBytes > 0 {
			// X-Total-Bytes is the *source* total (uncompressed).
			// The response body's Content-Length is unknown for
			// tar.gz / zip so we leave that header off; clients
			// that want a percentage use this header instead.
			c.Writer.Header().Set("X-Total-Bytes", fmt.Sprintf("%d", totalBytes))
		}
		c.Status(http.StatusOK)
		c.Writer.Flush()

		// Two counters tick together while we stream chunks:
		//   wireBytes   — bytes written to the HTTP response (post-
		//                 gzip / deflate). Drives compression-ratio
		//                 and network-speed display.
		//   sourceBytes — uncompressed bytes the agent has read off
		//                 disk by the time it stamped this chunk;
		//                 carried in FileChunk.source_bytes_so_far.
		//                 Drives the progress percentage.
		var wireBytes int64
		var sourceBytes int64
		lastFlushWire := int64(0)
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
				finalizeTransfer(deps, ft, finalStatus, sourceBytes, wireBytes, errMsg, body.Paths)
				return
			}
			if ch.SourceBytesSoFar > sourceBytes {
				sourceBytes = ch.SourceBytesSoFar
			}
			if len(ch.Data) > 0 {
				if _, werr := c.Writer.Write(ch.Data); werr != nil {
					// Client disconnect mid-stream — treat as cancel.
					finalizeTransfer(deps, ft, storage.TransferStatusCanceled, sourceBytes, wireBytes, werr.Error(), body.Paths)
					return
				}
				atomic.AddInt64(&wireBytes, int64(len(ch.Data)))
				now := time.Now()
				if wireBytes-lastFlushWire >= progressFlushBytes ||
					now.Sub(lastFlushAt) >= progressFlushInterval {
					_ = deps.Recorder.UpdateProgress(c.Request.Context(), transferID, sourceBytes, wireBytes, totalBytes)
					ft.BytesTransferred = sourceBytes
					ft.WireBytes = wireBytes
					broadcastTransfer(deps, ft, body.Paths)
					lastFlushWire = wireBytes
					lastFlushAt = now
					c.Writer.Flush()
				}
			}
			if ch.Eof {
				if ch.Error != "" {
					finalizeTransfer(deps, ft, storage.TransferStatusFailed, sourceBytes, wireBytes, ch.Error, body.Paths)
					return
				}
				finalizeTransfer(deps, ft, storage.TransferStatusDone, sourceBytes, wireBytes, "", body.Paths)
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

func finalizeTransfer(deps FileArchiveDeps, ft *storage.FileTransfer, status string, bytes, wireBytes int64, errMsg string, paths []string) {
	at := time.Now().UTC()
	ft.Status = status
	ft.BytesTransferred = bytes
	ft.WireBytes = wireBytes
	ft.ErrorMessage = errMsg
	ft.FinishedAt = &at
	if err := deps.Recorder.Finish(context.Background(), ft.ID, status, bytes, wireBytes, errMsg, at); err != nil {
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
				"wire_bytes":        wireBytes,
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
		WireBytes:        ft.WireBytes,
		TotalBytes:       ft.TotalBytes,
		ErrorMessage:     ft.ErrorMessage,
		StartedAt:        ft.StartedAt.Format(time.RFC3339Nano),
	}
	if ft.FinishedAt != nil {
		ev.FinishedAt = ft.FinishedAt.Format(time.RFC3339Nano)
	}
	deps.Broadcaster.Broadcast(EventTypeFileTransferUpdated, ev)
}

// v2FileDownloadTracked is the tracked counterpart of the legacy
// streaming /fs/read handler. Wire shape is identical (the agent
// returns FileReadResponse + a stream of FileChunk frames; we pipe
// them straight to the HTTP response body). The two differences:
//
//   1. We open a file_transfers row before the first byte flows so
//      operators see the download in the global Transfers drawer
//      while it's running. total_bytes comes from the agent's
//      FileReadResponse.Size header so the progress bar is sized.
//   2. Bytes-out tick into UpdateProgress + a WS broadcast on the
//      same size+time triggers the archive download uses, so the
//      drawer's progress bar updates live (and the operator can
//      cancel mid-stream from the UI without aborting the browser
//      tab).
//
// When the recorder is nil (tests register without it) the handler
// degrades to a plain streaming pass-through so the existing wire
// contract still holds.
func v2FileDownloadTracked(deps FileArchiveDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := lookupAgent(c, deps.Service)
		if !ok {
			return
		}
		path := c.Query("path")
		if path == "" {
			c.String(http.StatusBadRequest, "path query param required")
			return
		}

		// Range branch: a single `bytes=A-B` / `bytes=A-` / `bytes=-N`
		// request is served as 206 Partial Content and bypasses the
		// TransferRecorder — otherwise a single video preview that
		// seeks 50 times would explode the file_transfers drawer
		// into 50 rows. Multi-range or malformed Range headers fall
		// through to the full 200 path so badly-behaved clients
		// still get bytes (the response just isn't ranged).
		if rngHdr := c.GetHeader("Range"); rngHdr != "" {
			if spec, ok := parseSingleByteRange(rngHdr); ok {
				serveFsReadRange(c, deps, sess, c.Param("agent_id"), path, spec)
				return
			}
		}

		offset, _ := parseInt64Query(c, "offset")
		length, _ := parseInt64Query(c, "length")

		projectID := c.Param("pid")
		agentID := c.Param("agent_id")
		userIDStr := ""
		if claims, ok := ClaimsFromContext(c); ok && claims != nil {
			userIDStr = claims.UserID
		}

		meta, _ := proto.Marshal(&v2pb.FileReadRequest{
			Path: path, Offset: offset, Length: length,
		})
		stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_READ, meta,
			"fs-read-"+agentID)
		if err != nil {
			c.String(http.StatusBadGateway, "open stream: %s", err)
			return
		}
		defer func() { _ = stream.Close() }()

		var hdr v2pb.FileReadResponse
		if err := link.ReadFrame(stream, &hdr); err != nil {
			c.String(http.StatusBadGateway, "read header: %s", err)
			return
		}
		if hdr.Error != "" {
			c.String(http.StatusBadGateway, "agent: %s", hdr.Error)
			return
		}

		// Total bytes is only meaningful for full downloads —
		// offset/length slices have a smaller effective size we
		// don't pre-compute. Mirrors the un-tracked v2FileDownload's
		// Content-Length behaviour.
		var totalBytes int64
		if hdr.Size > 0 && length == 0 && offset == 0 {
			totalBytes = hdr.Size
		}

		var ft *storage.FileTransfer
		var transferID string
		var streamCtx context.Context = c.Request.Context()
		if deps.Recorder != nil {
			transferID = deps.IDGenerator()
			pathsJSON := pathsJSONOne(path)
			now := time.Now().UTC()
			ft = &storage.FileTransfer{
				ID:         transferID,
				ProjectID:  projectID,
				HostID:     resolveHostID(c.Request.Context(), deps, agentID),
				UserID:     userIDStr,
				Direction:  storage.TransferDirectionDownload,
				Kind:       storage.TransferKindFile,
				Format:     "",
				PathsJSON:  pathsJSON,
				Status:     storage.TransferStatusRunning,
				TotalBytes: totalBytes,
				StartedAt:  now,
			}
			if err := deps.Recorder.Create(c.Request.Context(), ft); err != nil {
				log.Warn("file_transfer create (download): %v", err)
			}
			broadcastTransfer(deps, ft, []string{path})

			var cancel context.CancelFunc
			streamCtx, cancel = context.WithCancel(c.Request.Context())
			defer cancel()
			if deps.Cancels != nil {
				deps.Cancels.Register(transferID, cancel)
				defer deps.Cancels.Unregister(transferID)
			}
			go func() {
				<-streamCtx.Done()
				if dl, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
					_ = dl.SetReadDeadline(time.Now().Add(-time.Second))
				}
				_ = stream.Close()
			}()
		}

		c.Writer.Header().Set("Content-Type", "application/octet-stream")
		// Advertise Range support so well-behaved clients (pdf.js,
		// video.js) opt into Range on the second request even though
		// this first response was 200.
		c.Writer.Header().Set("Accept-Ranges", "bytes")
		if totalBytes > 0 {
			c.Writer.Header().Set("Content-Length",
				fmt.Sprintf("%d", totalBytes))
		}
		if transferID != "" {
			c.Writer.Header().Set("X-Transfer-Id", transferID)
		}
		c.Status(http.StatusOK)

		// Single-file downloads pass through unchanged — no
		// compression on top of FileChunk frames — so wire bytes
		// equal source bytes (and equal what the client receives).
		// We track one counter and report it as both fields so the
		// frontend's compression-ratio + speed math degrades to
		// 1.0× with no special-casing.
		var bytesOut int64
		lastFlushBytes := int64(0)
		lastFlushAt := time.Now()
		for {
			var ch v2pb.FileChunk
			if err := link.ReadFrame(stream, &ch); err != nil {
				if ft != nil {
					if errors.Is(err, io.EOF) {
						finalizeFileDownload(deps, ft, storage.TransferStatusDone, bytesOut, "", path)
					} else {
						finalizeFileDownload(deps, ft,
							statusFromCtx(streamCtx, storage.TransferStatusFailed),
							bytesOut, err.Error(), path)
					}
				}
				return
			}
			if len(ch.Data) > 0 {
				if _, werr := c.Writer.Write(ch.Data); werr != nil {
					if ft != nil {
						finalizeFileDownload(deps, ft, storage.TransferStatusCanceled,
							bytesOut, werr.Error(), path)
					}
					return
				}
				bytesOut += int64(len(ch.Data))
				now := time.Now()
				if ft != nil && (bytesOut-lastFlushBytes >= progressFlushBytes ||
					now.Sub(lastFlushAt) >= progressFlushInterval) {
					_ = deps.Recorder.UpdateProgress(c.Request.Context(), transferID, bytesOut, bytesOut, totalBytes)
					ft.BytesTransferred = bytesOut
					ft.WireBytes = bytesOut
					broadcastTransfer(deps, ft, []string{path})
					lastFlushBytes = bytesOut
					lastFlushAt = now
				}
				c.Writer.Flush()
			}
			if ch.Eof {
				if ft != nil {
					finalizeFileDownload(deps, ft, storage.TransferStatusDone, bytesOut, "", path)
				}
				return
			}
		}
	}
}

// finalizeFileDownload mirrors finalizeTransfer but emits a different
// activity action so the unified audit log distinguishes single-file
// downloads from archive downloads at a glance.
func finalizeFileDownload(deps FileArchiveDeps, ft *storage.FileTransfer, status string, bytes int64, errMsg string, path string) {
	at := time.Now().UTC()
	ft.Status = status
	ft.BytesTransferred = bytes
	ft.WireBytes = bytes
	ft.ErrorMessage = errMsg
	ft.FinishedAt = &at
	if err := deps.Recorder.Finish(context.Background(), ft.ID, status, bytes, bytes, errMsg, at); err != nil {
		log.Warn("file_transfer finish (download): %v", err)
	}
	broadcastTransfer(deps, ft, []string{path})
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
			Action:      "download.file",
			TargetType:  "agent",
			TargetID:    ft.HostID,
			TargetLabel: path,
			Outcome:     outcome,
			Error:       errMsg,
			Meta: map[string]any{
				"transfer_id":       ft.ID,
				"bytes_transferred": bytes,
				"status":            status,
			},
			At: at,
		})
	}
}

// parseInt64Query reads an integer query param; missing / unparsable
// values yield zero. Mirrors the original v2FileDownload helper.
func parseInt64Query(c *gin.Context, key string) (int64, bool) {
	s := c.Query(key)
	if s == "" {
		return 0, false
	}
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int64(ch-'0')
	}
	return n, true
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

// resolveHostID maps an agentID to its host UUID via the optional
// HostLookup. file_transfers.host_id used to (incorrectly) hold the
// agent_id, which broke the per-host filter on the API. With the
// lookup wired in production this returns the real host UUID; when
// the lookup isn't available (legacy callers, tests that don't care
// about host scoping) we keep storing the agent_id so the column
// stays non-empty.
//
// On lookup error we log and fall back to the agent_id rather than
// 500'ing the download — a missing host row is recoverable later
// and the operator's transfer shouldn't be aborted by a metadata
// inconsistency.
func resolveHostID(ctx context.Context, deps FileArchiveDeps, agentID string) string {
	if deps.Hosts == nil {
		return agentID
	}
	host, err := deps.Hosts.GetByAgentID(ctx, agentID)
	if err != nil || host == nil {
		log.Warn("file_transfer host_id resolve: agent_id=%s err=%v", agentID, err)
		return agentID
	}
	return host.ID
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
