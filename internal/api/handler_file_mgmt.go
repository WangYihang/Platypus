package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// fileEntryDTO is the JSON shape the REST layer returns to the UI. We
// expose a stable camelCase schema here rather than leaking protobuf
// field names so frontend code keeps working if we ever swap the wire
// format.
type fileEntryDTO struct {
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	Mode          uint32 `json:"mode"`
	ModTimeUnix   int64  `json:"modTimeUnix"`
	IsDir         bool   `json:"isDir"`
	IsSymlink     bool   `json:"isSymlink"`
	SymlinkTarget string `json:"symlinkTarget,omitempty"`
	Err           string `json:"error,omitempty"`
}

func toEntryDTO(e *agentpb.FileEntry) fileEntryDTO {
	if e == nil {
		return fileEntryDTO{}
	}
	return fileEntryDTO{
		Name:          e.Name,
		Size:          e.Size,
		Mode:          e.Mode,
		ModTimeUnix:   e.ModTimeUnix,
		IsDir:         e.IsDir,
		IsSymlink:     e.IsSymlink,
		SymlinkTarget: e.SymlinkTarget,
		Err:           e.Error,
	}
}

// parseOctalMode accepts either an octal string ("755", "0755") or a
// decimal fallback. Empty returns 0 — handlers decide whether that's
// acceptable (Mkdir accepts 0 and substitutes 0755; Chmod rejects it).
func parseOctalMode(raw string) (uint32, error) {
	if raw == "" {
		return 0, nil
	}
	// strconv.ParseUint with base=0 auto-detects 0-prefixed octal AND
	// decimals, but we want bare "755" to be octal. Force base 8.
	n, err := strconv.ParseUint(raw, 8, 32)
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}

// ListDirHandler returns a page of directory entries.
//
// @Summary     List directory
// @Description List entries in a directory on the managed host, paged by offset+limit.
// @Tags        files
// @Produce     json
// @Security    BearerAuth
// @Param       id     path      string  true   "Session hash"
// @Param       path   query     string  true   "Absolute directory path"
// @Param       offset query     integer false  "Entry offset (0-based)" default(0)
// @Param       limit  query     integer false  "Max entries to return; 0 = server default" default(0)
// @Success     200    {object}  listDirResponse
// @Failure     400    {object}  errorResponse
// @Failure     404    {object}  errorResponse
// @Failure     502    {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files/list [get]
func ListDirHandler(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "0"), 10, 64)

	client := core.FindAgentClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	start := time.Now().UTC()
	entries, total, eof, err := client.ListDir(path, offset, limit)
	dur := time.Since(start).Milliseconds()
	in := ActivityInput{
		Category:    storage.CategoryFile,
		Action:      "file.list",
		TargetType:  "session",
		TargetID:    client.Hash,
		TargetLabel: path,
		SessionID:   client.Hash,
		DurationMs:  &dur,
		At:          start,
		Meta: map[string]any{
			"path":   path,
			"offset": offset,
			"limit":  limit,
			"total":  total,
			"page":   len(entries),
		},
	}
	if err != nil {
		in.Outcome = storage.OutcomeError
		in.Error = err.Error()
		RecordActivity(c, in)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	RecordActivity(c, in)

	out := make([]fileEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, toEntryDTO(e))
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"entries": out,
		"total":   total,
		"eof":     eof,
	})
}

// StatHandler returns metadata for a single path.
//
// @Summary     Stat path
// @Description Return metadata (size/mode/mtime/type) for a single file or directory.
// @Tags        files
// @Produce     json
// @Security    BearerAuth
// @Param       id    path      string  true  "Session hash"
// @Param       path  query     string  true  "Absolute path"
// @Success     200   {object}  statResponse
// @Failure     400   {object}  errorResponse
// @Failure     404   {object}  errorResponse
// @Failure     502   {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files/stat [get]
func StatHandler(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}
	client := core.FindAgentClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	start := time.Now().UTC()
	entry, err := client.Stat(path)
	dur := time.Since(start).Milliseconds()
	in := ActivityInput{
		Category:    storage.CategoryFile,
		Action:      "file.stat2",
		TargetType:  "session",
		TargetID:    client.Hash,
		TargetLabel: path,
		SessionID:   client.Hash,
		DurationMs:  &dur,
		At:          start,
		Meta:        map[string]any{"path": path},
	}
	if err != nil {
		in.Outcome = storage.OutcomeError
		in.Error = err.Error()
		RecordActivity(c, in)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	RecordActivity(c, in)
	c.JSON(http.StatusOK, gin.H{"status": true, "entry": toEntryDTO(entry)})
}

// DeleteFileHandler removes a file or (with recursive=true) a directory.
//
// @Summary     Delete path
// @Description Remove a file, empty directory, or — with recursive=true — a directory subtree.
// @Tags        files
// @Produce     json
// @Security    BearerAuth
// @Param       id        path      string   true  "Session hash"
// @Param       path      query     string   true  "Absolute path to remove"
// @Param       recursive query     boolean  false "Remove directory contents" default(false)
// @Success     200       {object}  statusResponse
// @Failure     400       {object}  errorResponse
// @Failure     404       {object}  errorResponse
// @Failure     502       {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files [delete]
func DeleteFileHandler(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}
	recursive := c.DefaultQuery("recursive", "false") == "true"
	client := core.FindAgentClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	start := time.Now().UTC()
	err := client.Delete(path, recursive)
	dur := time.Since(start).Milliseconds()
	in := ActivityInput{
		Category:    storage.CategoryFile,
		Action:      "file.delete",
		TargetType:  "session",
		TargetID:    client.Hash,
		TargetLabel: path,
		SessionID:   client.Hash,
		DurationMs:  &dur,
		At:          start,
		Meta:        map[string]any{"path": path, "recursive": recursive},
	}
	if err != nil {
		in.Outcome = storage.OutcomeError
		in.Error = err.Error()
		RecordActivity(c, in)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	RecordActivity(c, in)
	c.JSON(http.StatusOK, gin.H{"status": true})
}

type renameBody struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to"   binding:"required"`
}

// RenameFileHandler moves (renames) a file or directory on the agent.
//
// @Summary     Rename / move path
// @Description Rename or move a file or directory. Cross-filesystem renames return an EXDEV error from the agent — the client may fall back to copy+delete.
// @Tags        files
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path      string      true  "Session hash"
// @Param       body  body      renameBody  true  "Source and destination absolute paths"
// @Success     200   {object}  statusResponse
// @Failure     400   {object}  errorResponse
// @Failure     404   {object}  errorResponse
// @Failure     502   {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files/rename [post]
func RenameFileHandler(c *gin.Context) {
	hash := c.Param("id")
	var body renameBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client := core.FindAgentClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	start := time.Now().UTC()
	err := client.Rename(body.From, body.To)
	dur := time.Since(start).Milliseconds()
	in := ActivityInput{
		Category:    storage.CategoryFile,
		Action:      "file.rename",
		TargetType:  "session",
		TargetID:    client.Hash,
		TargetLabel: body.From,
		SessionID:   client.Hash,
		DurationMs:  &dur,
		At:          start,
		Meta:        map[string]any{"from": body.From, "to": body.To},
	}
	if err != nil {
		in.Outcome = storage.OutcomeError
		in.Error = err.Error()
		RecordActivity(c, in)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	RecordActivity(c, in)
	c.JSON(http.StatusOK, gin.H{"status": true})
}

// MkdirHandler creates a directory on the agent.
//
// @Summary     Create directory
// @Description Create a directory. With parents=true, creates intermediate parents like mkdir -p. Mode is parsed as octal ("755").
// @Tags        files
// @Produce     json
// @Security    BearerAuth
// @Param       id      path      string   true   "Session hash"
// @Param       path    query     string   true   "Absolute directory path"
// @Param       parents query     boolean  false  "Create parent directories" default(false)
// @Param       mode    query     string   false  "Octal mode (default 755)" default("755")
// @Success     200     {object}  statusResponse
// @Failure     400     {object}  errorResponse
// @Failure     404     {object}  errorResponse
// @Failure     502     {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files/mkdir [post]
func MkdirHandler(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}
	parents := c.DefaultQuery("parents", "false") == "true"
	mode, err := parseOctalMode(c.DefaultQuery("mode", ""))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode (expected octal)"})
		return
	}
	client := core.FindAgentClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	start := time.Now().UTC()
	err = client.Mkdir(path, parents, mode)
	dur := time.Since(start).Milliseconds()
	in := ActivityInput{
		Category:    storage.CategoryFile,
		Action:      "file.mkdir",
		TargetType:  "session",
		TargetID:    client.Hash,
		TargetLabel: path,
		SessionID:   client.Hash,
		DurationMs:  &dur,
		At:          start,
		Meta:        map[string]any{"path": path, "parents": parents, "mode": mode},
	}
	if err != nil {
		in.Outcome = storage.OutcomeError
		in.Error = err.Error()
		RecordActivity(c, in)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	RecordActivity(c, in)
	c.JSON(http.StatusOK, gin.H{"status": true})
}

// ChmodHandler changes the permission bits on a path.
//
// @Summary     Change mode
// @Description Set permission bits on a file or directory. Mode is octal ("644", "755"). On Windows, only the owner-write bit is meaningful.
// @Tags        files
// @Produce     json
// @Security    BearerAuth
// @Param       id    path      string   true  "Session hash"
// @Param       path  query     string   true  "Absolute path"
// @Param       mode  query     string   true  "Octal mode (e.g. 644)"
// @Success     200   {object}  statusResponse
// @Failure     400   {object}  errorResponse
// @Failure     404   {object}  errorResponse
// @Failure     502   {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files/chmod [post]
func ChmodHandler(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}
	modeStr := c.Query("mode")
	if modeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode query parameter required"})
		return
	}
	mode, err := parseOctalMode(modeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode (expected octal)"})
		return
	}
	client := core.FindAgentClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	start := time.Now().UTC()
	err = client.Chmod(path, mode)
	dur := time.Since(start).Milliseconds()
	in := ActivityInput{
		Category:    storage.CategoryFile,
		Action:      "file.chmod",
		TargetType:  "session",
		TargetID:    client.Hash,
		TargetLabel: path,
		SessionID:   client.Hash,
		DurationMs:  &dur,
		At:          start,
		Meta:        map[string]any{"path": path, "mode": mode},
	}
	if err != nil {
		in.Outcome = storage.OutcomeError
		in.Error = err.Error()
		RecordActivity(c, in)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	RecordActivity(c, in)
	c.JSON(http.StatusOK, gin.H{"status": true})
}
