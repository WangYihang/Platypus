package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
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
	Host string `json:"host" binding:"required"`
	Port int    `json:"port" binding:"required"`
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
// @Description Opens a TLS ingress port where managed-host agents dial in.
// @Tags        listeners
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body     createListenerRequest true "Bind address"
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
	srv := core.CreateTCPServer(req.Host, uint16(req.Port), "", true, "", "")
	if srv == nil {
		RecordActivity(c, ActivityInput{
			Category: storage.CategoryListener,
			Action:   "listener.create",
			Outcome:  storage.OutcomeError,
			Error:    "failed to start listener",
			Meta:     map[string]any{"host": req.Host, "port": req.Port},
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start listener"})
		return
	}
	go (*srv).Run()
	RecordActivity(c, ActivityInput{
		Category:    storage.CategoryListener,
		Action:      "listener.create",
		TargetType:  "listener",
		TargetID:    (*srv).Hash,
		TargetLabel: listenerLabel(srv),
		Meta: map[string]any{
			"host": req.Host,
			"port": req.Port,
			"hash": (*srv).Hash,
		},
	})
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
			label := listenerLabel(srv)
			core.DeleteServer(srv)
			RecordActivity(c, ActivityInput{
				Category:    storage.CategoryListener,
				Action:      "listener.delete",
				TargetType:  "listener",
				TargetID:    hash,
				TargetLabel: label,
			})
			c.Status(http.StatusNoContent)
			return
		}
	}
	RecordActivity(c, ActivityInput{
		Category:   storage.CategoryListener,
		Action:     "listener.delete",
		TargetType: "listener",
		TargetID:   hash,
		Outcome:    storage.OutcomeDenied,
		Error:      "listener not found",
	})
	c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
}

// listenerLabel builds a human-readable "host:port" string for use as
// a TargetLabel on listener activities.
func listenerLabel(srv *core.TCPServer) string {
	if srv == nil {
		return ""
	}
	return srv.Host + ":" + strconvUint16(srv.Port)
}

// strconvUint16 avoids pulling in strconv for a single call site; the
// existing code in this file doesn't import it.
func strconvUint16(v uint16) string {
	if v == 0 {
		return "0"
	}
	var buf [6]byte
	n := len(buf)
	for v > 0 {
		n--
		buf[n] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[n:])
}

// ListenerSessionsV1 lists every session attached to a listener.
//
// @Summary     List sessions on a listener
// @Description Returns every agent session that entered through this listener.
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
		for _, cl := range srv.AgentClients {
			sessions = append(sessions, cl)
		}
		c.JSON(http.StatusOK, sessionsListResponse{Sessions: sessions})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
}
