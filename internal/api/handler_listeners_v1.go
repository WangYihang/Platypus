package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
)

// v1 listener endpoints mirror the legacy /api/server/* routes with a JSON
// request shape (no more application/x-www-form-urlencoded), a consistent
// {listeners:[]} list envelope, and proper 4xx/5xx status codes. The legacy
// routes stay alive behind Deprecation headers.

// listenersListResponse is the shape of GET /api/v1/listeners.
type listenersListResponse struct {
	Listeners []interface{} `json:"listeners"`
}

// createListenerRequest is the JSON body for POST /api/v1/listeners.
type createListenerRequest struct {
	Host      string `json:"host" binding:"required"`
	Port      int    `json:"port" binding:"required"`
	Encrypted bool   `json:"encrypted"`
}

// ListListenersV1 returns every configured TCP listener as a flat array.
//
// @Summary     List listeners
// @Description Returns every TCP listener currently registered. Replaces the legacy /api/server, which additionally embedded a Distributor snapshot; consumers that need the distributor should fetch it separately (future endpoint).
// @Tags        listeners
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} listenersListResponse
// @Router      /api/v1/listeners [get]
func ListListenersV1(c *gin.Context) {
	out := []interface{}{}
	for _, srv := range core.GetServers() {
		out = append(out, srv)
	}
	c.JSON(http.StatusOK, listenersListResponse{Listeners: out})
}

// GetListenerV1 fetches one listener by hash.
//
// @Summary     Get listener
// @Description Returns a single listener by hash. Replaces the legacy /api/server/{hash}.
// @Tags        listeners
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "Listener hash"
// @Success     200 {object} core.TCPServer
// @Failure     404 {object} errorResponse
// @Router      /api/v1/listeners/{id} [get]
func GetListenerV1(c *gin.Context) {
	hash := c.Param("id")
	for _, srv := range core.GetServers() {
		if srv.Hash == hash {
			c.JSON(http.StatusOK, srv)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
}

// CreateListenerV1 starts a new listener. Replaces the form-encoded
// POST /api/server with a JSON request.
//
// @Summary     Create listener
// @Description Opens a reverse-shell listener. encrypted=true yields a TLS+proto Termite listener; false yields a plain raw-TCP shell listener.
// @Tags        listeners
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     createListenerRequest true "Bind address and encryption mode"
// @Success     201  {object} core.TCPServer
// @Failure     400  {object} errorResponse
// @Failure     500  {object} errorResponse
// @Router      /api/v1/listeners [post]
func CreateListenerV1(c *gin.Context) {
	var req createListenerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host and port are required (JSON body)"})
		return
	}
	if req.Port <= 0 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "port must be 1-65535"})
		return
	}
	srv := core.CreateTCPServer(req.Host, uint16(req.Port), "", req.Encrypted, true, "", "")
	if srv == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start listener"})
		return
	}
	go (*srv).Run()
	c.JSON(http.StatusCreated, srv)
}

// DeleteListenerV1 tears down a listener.
//
// @Summary     Delete listener
// @Description Stops a listener and deregisters it. Returns 204 on success, 404 if no such listener. Replaces DELETE /api/server/{hash}.
// @Tags        listeners
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "Listener hash"
// @Success     204 "No Content"
// @Failure     404 {object} errorResponse
// @Router      /api/v1/listeners/{id} [delete]
func DeleteListenerV1(c *gin.Context) {
	hash := c.Param("id")
	for _, srv := range core.GetServers() {
		if srv.Hash == hash {
			core.DeleteServer(srv)
			c.Status(http.StatusNoContent)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
}

// ListenerSessionsV1 lists every session attached to a listener.
//
// @Summary     List sessions on a listener
// @Description Returns every TCP + Termite session that entered through this listener. Replaces the legacy /api/server/{hash}/client.
// @Tags        listeners
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "Listener hash"
// @Success     200 {object} sessionsListResponse
// @Failure     404 {object} errorResponse
// @Router      /api/v1/listeners/{id}/sessions [get]
func ListenerSessionsV1(c *gin.Context) {
	hash := c.Param("id")
	for _, srv := range core.GetServers() {
		if srv.Hash != hash {
			continue
		}
		sessions := []interface{}{}
		for _, cl := range srv.Clients {
			sessions = append(sessions, cl)
		}
		for _, cl := range srv.TermiteClients {
			sessions = append(sessions, cl)
		}
		c.JSON(http.StatusOK, sessionsListResponse{Sessions: sessions})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
}
