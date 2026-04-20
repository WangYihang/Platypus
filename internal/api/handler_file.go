package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
)

// GetFileSize returns the size in bytes of a file on the victim.
//
// @Summary     File size
// @Description Stat a remote file and return its size in bytes. Requires the session to be connected.
// @Tags        files
// @Produce     json
// @Security    BearerAuth
// @Param       id    path      string  true  "Session hash"
// @Param       path  query     string  true  "Absolute path to the file"
// @Success     200   {object}  map[string]any "status + size"
// @Failure     400   {object}  errorResponse
// @Failure     404   {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files/size [get]
func GetFileSize(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}

	if client := core.FindTermiteClientByHash(hash); client != nil {
		size, err := client.FileSize(path)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": true, "size": size})
		return
	}

	if client := core.FindTCPClientByHash(hash); client != nil {
		size, err := client.FileSize(path)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": true, "size": size})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// ReadFile streams part of a remote file.
//
// @Summary     Read file
// @Description Stream a slice of a remote file as application/octet-stream. Omit size=0 to read the whole file; otherwise read [offset, offset+size).
// @Tags        files
// @Produce     application/octet-stream
// @Security    BearerAuth
// @Param       id     path      string  true   "Session hash"
// @Param       path   query     string  true   "Absolute path to the file"
// @Param       offset query     integer false  "Byte offset to start reading from" default(0)
// @Param       size   query     integer false  "Number of bytes to read; 0 = whole file" default(0)
// @Success     200    {file}    binary
// @Failure     400    {object}  errorResponse
// @Failure     404    {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files [get]
func ReadFile(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	size, _ := strconv.ParseInt(c.DefaultQuery("size", "0"), 10, 64)

	if client := core.FindTermiteClientByHash(hash); client != nil {
		var data []byte
		var err error
		if size == 0 {
			data, err = client.ReadFile(path)
		} else {
			data, err = client.ReadFileEx(path, offset, size)
		}
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": false, "error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", data)
		return
	}

	if client := core.FindTCPClientByHash(hash); client != nil {
		content, err := client.ReadFileEx(path, int(offset), int(size))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": false, "error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", []byte(content))
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// WriteFile uploads raw bytes to a remote file.
//
// @Summary     Write file
// @Description Upload bytes to a remote path. Use append=true to chunk large uploads. Only Termite sessions support file upload.
// @Tags        files
// @Accept      application/octet-stream
// @Produce     json
// @Security    BearerAuth
// @Param       id     path      string  true   "Session hash (must be a Termite session)"
// @Param       path   query     string  true   "Absolute path to write to"
// @Param       append query     boolean false  "If true, appends to the file instead of truncating" default(false)
// @Param       body   body      string  true   "Raw file bytes"
// @Success     200    {object}  map[string]any "status + bytes_written"
// @Failure     400    {object}  errorResponse
// @Failure     404    {object}  errorResponse
// @Router      /api/v1/sessions/{id}/files [post]
func WriteFile(c *gin.Context) {
	hash := c.Param("id")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}
	appendMode := c.DefaultQuery("append", "false") == "true"

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	if client := core.FindTermiteClientByHash(hash); client != nil {
		var n int
		if appendMode {
			n, err = client.WriteFileEx(path, data)
		} else {
			n, err = client.WriteFile(path, data)
		}
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": true, "bytes_written": n})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "session not found (file upload requires Termite client)"})
}
