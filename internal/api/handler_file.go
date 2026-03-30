package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/gin-gonic/gin"
)

// GetFileSize handles GET /api/v1/sessions/:id/files/size?path=X
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

// ReadFile handles GET /api/v1/sessions/:id/files?path=X&offset=N&size=N
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

// WriteFile handles POST /api/v1/sessions/:id/files?path=X&append=true
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
