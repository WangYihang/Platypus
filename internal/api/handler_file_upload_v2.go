package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// v2FileUploadTracked is the tracked counterpart of v2FileUpload. The
// wire shape is the same (PUT body → agent FILE_WRITE stream), but
// every upload also:
//
//   1. creates a file_transfers row in 'running' state;
//   2. registers a cancel func so /transfers/:id/cancel can tear it
//      down mid-stream;
//   3. ticks progress (DB + WS broadcast) every progressFlushBytes /
//      progressFlushInterval;
//   4. finalises to done/failed/canceled and writes an activity row.
//
// total_bytes query param sizes the progress bar before the body
// starts flowing. When 0 the progress bar stays indeterminate; the
// final row still records the actual bytes received.
func v2FileUploadTracked(deps FileArchiveDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := lookupAgent(c, deps.Service)
		if !ok {
			return
		}
		dstPath := c.Query("path")
		if dstPath == "" {
			c.String(http.StatusBadRequest, "path query param required")
			return
		}
		appendMode := c.Query("append") == "true"
		mkdirs := c.Query("mkdirs") == "true"
		var totalBytes int64
		if v := c.Query("total_bytes"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				totalBytes = n
			}
		}

		projectID := c.Param("pid")
		agentID := c.Param("agent_id")
		userIDStr := ""
		if claims, ok := ClaimsFromContext(c); ok && claims != nil {
			userIDStr = claims.UserID
		}

		transferID := deps.IDGenerator()
		now := time.Now().UTC()
		ft := &storage.FileTransfer{
			ID:         transferID,
			ProjectID:  projectID,
			HostID:     resolveHostID(c.Request.Context(), deps, agentID),
			UserID:     userIDStr,
			Direction:  storage.TransferDirectionUpload,
			Kind:       storage.TransferKindFile,
			Format:     "",
			PathsJSON:  pathsJSONOne(dstPath),
			Status:     storage.TransferStatusRunning,
			TotalBytes: totalBytes,
			StartedAt:  now,
		}
		if err := deps.Recorder.Create(c.Request.Context(), ft); err != nil {
			log.Warn("file_transfer create (upload): %v", err)
		}
		broadcastTransfer(deps, ft, []string{dstPath})

		streamCtx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		deps.Cancels.Register(transferID, cancel)
		defer deps.Cancels.Unregister(transferID)

		// Surface the transfer id early so the client can drive UI
		// (progress / cancel) from the response headers even before
		// the body finishes flowing.
		c.Writer.Header().Set("X-Transfer-Id", transferID)

		meta, _ := proto.Marshal(&v2pb.FileWriteRequest{
			Path:   dstPath,
			Append: appendMode,
			Mkdirs: mkdirs,
		})
		stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_WRITE, meta,
			"fs-upload-"+transferID)
		if err != nil {
			finalizeUpload(deps, ft, storage.TransferStatusFailed, 0, err.Error(), dstPath)
			c.String(http.StatusBadGateway, "open stream: %s", err)
			return
		}
		// Mirror the archive-download cancel pattern. Three things on
		// streamCtx fire so cancel unwinds promptly regardless of
		// where the goroutine is parked:
		//   * past read deadline on the yamux stream — aborts in-flight
		//     link.ReadFrame on ack / result;
		//   * stream.Close() — same channel, idempotent;
		//   * Request.Body.Close() — aborts the in-flight Read against
		//     the inbound HTTP body so the upload loop can observe the
		//     cancel without polling.
		go func() {
			<-streamCtx.Done()
			if dl, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
				_ = dl.SetReadDeadline(time.Now().Add(-time.Second))
			}
			_ = stream.Close()
			if c.Request != nil && c.Request.Body != nil {
				_ = c.Request.Body.Close()
			}
		}()

		var ack v2pb.FileWriteResponse
		if err := link.ReadFrame(stream, &ack); err != nil {
			finalizeUpload(deps, ft, statusFromCtx(streamCtx, storage.TransferStatusFailed),
				0, err.Error(), dstPath)
			c.String(http.StatusBadGateway, "read ack: %s", err)
			return
		}
		if ack.Error != "" {
			finalizeUpload(deps, ft, storage.TransferStatusFailed, 0, ack.Error, dstPath)
			c.String(http.StatusBadGateway, "agent: %s", ack.Error)
			return
		}

		// Stream the body to the agent in fileUploadChunkSize blocks,
		// ticking progress on size+time triggers along the way. Body
		// reads run in a small pump goroutine so a cancel mid-Read
		// can abort the loop without waiting for the underlying conn
		// to deliver more bytes — net/http's Body.Close() doesn't
		// unblock an in-flight Read, so we need select-on-channel
		// instead of polling streamCtx between Reads.
		type readResult struct {
			data []byte
			err  error
		}
		readCh := make(chan readResult, 1)
		var pumpDone uint32
		pump := func() {
			defer atomic.StoreUint32(&pumpDone, 1)
			buf := make([]byte, fileUploadChunkSize)
			for {
				n, rerr := c.Request.Body.Read(buf)
				var data []byte
				if n > 0 {
					data = append([]byte(nil), buf[:n]...)
				}
				select {
				case readCh <- readResult{data: data, err: rerr}:
				case <-streamCtx.Done():
					return
				}
				if rerr != nil {
					return
				}
			}
		}
		go pump()

		var bytesIn int64
		lastFlushBytes := int64(0)
		lastFlushAt := time.Now()
	UPLOAD:
		for {
			select {
			case <-streamCtx.Done():
				finalizeUpload(deps, ft, storage.TransferStatusCanceled, bytesIn,
					streamCtx.Err().Error(), dstPath)
				// Connection: close + Hijack the conn so the client
				// sees the response immediately rather than waiting
				// for net/http to drain the still-flowing request
				// body. Without this, an HTTP/1.1 keep-alive client
				// blocks on Do() because the server can't free the
				// connection until the body is fully consumed.
				c.Header("Connection", "close")
				c.String(http.StatusRequestTimeout, "canceled")
				if hj, ok := c.Writer.(http.Hijacker); ok {
					if conn, _, hjerr := hj.Hijack(); hjerr == nil {
						_ = conn.Close()
					}
				}
				return
			case res := <-readCh:
				if len(res.data) > 0 {
					if werr := link.WriteFrame(stream, &v2pb.FileChunk{Data: res.data}); werr != nil {
						finalizeUpload(deps, ft, statusFromCtx(streamCtx, storage.TransferStatusFailed),
							bytesIn, werr.Error(), dstPath)
						c.String(http.StatusBadGateway, "write chunk: %s", werr)
						return
					}
					bytesIn += int64(len(res.data))
					now := time.Now()
					if bytesIn-lastFlushBytes >= progressFlushBytes ||
						now.Sub(lastFlushAt) >= progressFlushInterval {
						_ = deps.Recorder.UpdateProgress(c.Request.Context(), transferID, bytesIn, totalBytes)
						ft.BytesTransferred = bytesIn
						broadcastTransfer(deps, ft, []string{dstPath})
						lastFlushBytes = bytesIn
						lastFlushAt = now
					}
				}
				if res.err != nil {
					if !errors.Is(res.err, io.EOF) {
						finalizeUpload(deps, ft, statusFromCtx(streamCtx, storage.TransferStatusCanceled),
							bytesIn, res.err.Error(), dstPath)
						c.String(http.StatusBadRequest, "body read: %s", res.err)
						return
					}
					break UPLOAD
				}
			}
		}
		_ = pumpDone
		// Final eof chunk → tells the agent the body is fully drained.
		if err := link.WriteFrame(stream, &v2pb.FileChunk{Eof: true}); err != nil {
			finalizeUpload(deps, ft, statusFromCtx(streamCtx, storage.TransferStatusFailed),
				bytesIn, err.Error(), dstPath)
			c.String(http.StatusBadGateway, "write eof: %s", err)
			return
		}

		var res v2pb.FileWriteResult
		if err := link.ReadFrame(stream, &res); err != nil {
			finalizeUpload(deps, ft, statusFromCtx(streamCtx, storage.TransferStatusFailed),
				bytesIn, err.Error(), dstPath)
			c.String(http.StatusBadGateway, "read result: %s", err)
			return
		}
		if res.Error != "" {
			finalizeUpload(deps, ft, storage.TransferStatusFailed, bytesIn, res.Error, dstPath)
			c.String(http.StatusBadGateway, "agent: %s", res.Error)
			return
		}
		finalizeUpload(deps, ft, storage.TransferStatusDone, res.BytesWritten, "", dstPath)
		c.JSON(http.StatusOK, gin.H{
			"bytes_written": res.BytesWritten,
			"transfer_id":   transferID,
		})
	}
}

// finalizeUpload mirrors finalizeTransfer but emits a different
// activity action so the unified audit log distinguishes uploads
// from downloads at a glance.
func finalizeUpload(deps FileArchiveDeps, ft *storage.FileTransfer, status string, bytes int64, errMsg string, dst string) {
	at := time.Now().UTC()
	ft.Status = status
	ft.BytesTransferred = bytes
	ft.ErrorMessage = errMsg
	ft.FinishedAt = &at
	if err := deps.Recorder.Finish(context.Background(), ft.ID, status, bytes, errMsg, at); err != nil {
		log.Warn("file_transfer finish (upload): %v", err)
	}
	broadcastTransfer(deps, ft, []string{dst})
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
			Action:      "upload.file",
			TargetType:  "agent",
			TargetID:    ft.HostID,
			TargetLabel: dst,
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

// pathsJSONOne marshals a single-element ["<p>"] JSON array without
// dragging encoding/json in for one trivial encode. Path is wrapped
// in JSON-string-escape semantics: backslashes and quotes get
// escaped, control chars are dropped (paths shouldn't contain them).
func pathsJSONOne(p string) string {
	var b strings.Builder
	b.WriteString(`["`)
	for _, r := range p {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteString(`"]`)
	return b.String()
}

