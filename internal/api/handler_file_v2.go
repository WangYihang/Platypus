package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// fileUploadChunkSize bounds each inbound upload read before we
// wrap bytes into a FileChunk frame. Matches the agent-side reader
// chunk size so end-to-end paste-sized uploads aren't fragmented.
const fileUploadChunkSize = 256 * 1024

// RegisterV2FileRoutes mounts the v2 mutation file endpoints under
// the project-scoped agent group. Today this is just /fs/write; the
// matching /fs/read mount has moved to RegisterV2FileArchiveRoutes
// so single-file downloads share the same file_transfers tracking
// (row + WS progress + cancel) the archive download path uses.
//
// fs/write is operator-tier (a mutation). RequireAgentInProject
// prevents cross-project pivots via a forged agent_id under a
// project the caller is a member of.
func RegisterV2FileRoutes(engine *gin.Engine, svc *core.AgentLinkService, rbac *RBAC) {
	base := engine.Group("/api/v1/projects/:pid/agents/:agent_id")
	base.Use(rbac.RequireAuth())

	operator := base.Group("")
	operator.Use(
		rbac.RequireProjectRole("pid", user.RoleOperator),
		rbac.RequireAgentInProject("pid", "agent_id"),
	)
	operator.PUT("/fs/write", v2FileUpload(svc))
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
//
// Cross-project access is prevented upstream by RequireAgentInProject;
// this function only checks live presence in AgentLinkService.
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
