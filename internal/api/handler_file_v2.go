package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// fileUploadChunkSize bounds each inbound upload read before we
// wrap bytes into a FileChunk frame. Matches the agent-side reader
// chunk size so end-to-end paste-sized uploads aren't fragmented.
const fileUploadChunkSize = 256 * 1024

// RegisterV2FileRoutes mounts the v2 file endpoints. Both operate
// on a live agent looked up via AgentLinkService; missing agent →
// 404; agent-reported errors → 502 so the frontend knows the
// request reached the agent but the agent refused.
func RegisterV2FileRoutes(engine *gin.Engine, svc *core.AgentLinkService) {
	engine.GET("/api/v1/agents/:agent_id/fs/read", v2FileDownload(svc))
	engine.PUT("/api/v1/agents/:agent_id/fs/write", v2FileUpload(svc))
}

// v2FileDownload streams the remote file's content back to the
// caller as an octet-stream body. Piped straight from the yamux
// stream so memory usage stays proportional to the agent's chunk
// size, not the file size.
func v2FileDownload(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := lookupAgent(c, svc)
		if !ok {
			return
		}
		path := c.Query("path")
		if path == "" {
			c.String(http.StatusBadRequest, "path query param required")
			return
		}
		offset, _ := strconv.ParseInt(c.Query("offset"), 10, 64)
		length, _ := strconv.ParseInt(c.Query("length"), 10, 64)

		meta, _ := proto.Marshal(&v2pb.FileReadRequest{
			Path: path, Offset: offset, Length: length,
		})
		stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_READ, meta,
			"fs-read-"+c.Param("agent_id"))
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

		c.Writer.Header().Set("Content-Type", "application/octet-stream")
		if hdr.Size > 0 && length == 0 && offset == 0 {
			// Only set a known length on full downloads; offset /
			// length slices have a smaller effective size that we
			// don't pre-compute.
			c.Writer.Header().Set("Content-Length", strconv.FormatInt(hdr.Size, 10))
		}
		c.Status(http.StatusOK)

		for {
			var ch v2pb.FileChunk
			if err := link.ReadFrame(stream, &ch); err != nil {
				if !errors.Is(err, io.EOF) {
					log.Warn("v2 fs-read %s: mid-stream error: %v", path, err)
				}
				return
			}
			if len(ch.Data) > 0 {
				if _, err := c.Writer.Write(ch.Data); err != nil {
					return
				}
				c.Writer.Flush()
			}
			if ch.Eof {
				return
			}
		}
	}
}

// v2FileUpload streams the request body to the agent as FileChunk
// frames. Request body can be any size — reads happen in
// fileUploadChunkSize-sized blocks.
func v2FileUpload(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := lookupAgent(c, svc)
		if !ok {
			return
		}
		path := c.Query("path")
		if path == "" {
			c.String(http.StatusBadRequest, "path query param required")
			return
		}

		meta, _ := proto.Marshal(&v2pb.FileWriteRequest{
			Path:   path,
			Append: c.Query("append") == "true",
			Mkdirs: c.Query("mkdirs") == "true",
		})
		stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_WRITE, meta,
			"fs-write-"+c.Param("agent_id"))
		if err != nil {
			c.String(http.StatusBadGateway, "open stream: %s", err)
			return
		}
		defer func() { _ = stream.Close() }()

		var ack v2pb.FileWriteResponse
		if err := link.ReadFrame(stream, &ack); err != nil {
			c.String(http.StatusBadGateway, "read ack: %s", err)
			return
		}
		if ack.Error != "" {
			c.String(http.StatusBadGateway, "agent: %s", ack.Error)
			return
		}

		buf := make([]byte, fileUploadChunkSize)
		for {
			n, rerr := c.Request.Body.Read(buf)
			if n > 0 {
				if werr := link.WriteFrame(stream, &v2pb.FileChunk{
					Data: append([]byte(nil), buf[:n]...),
				}); werr != nil {
					c.String(http.StatusBadGateway, "write chunk: %s", werr)
					return
				}
			}
			if rerr != nil {
				break
			}
		}
		// Final eof chunk.
		if err := link.WriteFrame(stream, &v2pb.FileChunk{Eof: true}); err != nil {
			c.String(http.StatusBadGateway, "write eof: %s", err)
			return
		}

		var res v2pb.FileWriteResult
		if err := link.ReadFrame(stream, &res); err != nil {
			c.String(http.StatusBadGateway, "read result: %s", err)
			return
		}
		if res.Error != "" {
			c.String(http.StatusBadGateway, "agent: %s", res.Error)
			return
		}
		c.JSON(http.StatusOK, gin.H{"bytes_written": res.BytesWritten})
	}
}

// lookupAgent pulls agent_id from the gin context, fetches its
// session, and writes the appropriate HTTP error on miss. Returns
// (session, true) on success; caller proceeds. On false it's
// already written the response — caller returns immediately.
func lookupAgent(c *gin.Context, svc *core.AgentLinkService) (*link.Session, bool) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.String(http.StatusBadRequest, "agent_id required")
		return nil, false
	}
	sess, ok := svc.Get(agentID)
	if !ok {
		c.String(http.StatusNotFound, "agent %s not connected", agentID)
		return nil, false
	}
	return sess, true
}
