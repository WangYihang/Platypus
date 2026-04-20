package api

import (
	"net/http"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/gin-gonic/gin"
)

// createTunnelRequest is the typed mirror of the POST body for swag.
type createTunnelRequest struct {
	// Mode is one of "pull", "push", "dynamic", "internet".
	Mode string `json:"mode" binding:"required"`
	// SrcAddress is required for pull/push/internet modes. Ignored for dynamic.
	SrcAddress string `json:"src_address"`
	// DstAddress is required for pull/push/internet modes. Ignored for dynamic.
	DstAddress string `json:"dst_address"`
}

// CreateTunnel opens a tunnel through an encrypted session.
//
// @Summary     Create tunnel
// @Description Open a tunnel on a Termite session. Modes: pull (agent binds dst, forwards to src), push (operator binds src, agent dials dst), dynamic (agent runs a SOCKS5 server), internet (server runs SOCKS5 at src, agent proxies to dst).
// @Tags        tunnels
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path      string              true "Session hash (Termite only)"
// @Param       body body      createTunnelRequest true "Mode + addresses"
// @Success     200  {object}  map[string]any
// @Failure     400  {object}  errorResponse
// @Failure     404  {object}  errorResponse
// @Failure     409  {object}  errorResponse
// @Failure     500  {object}  errorResponse
// @Router      /api/v1/sessions/{id}/tunnels [post]
func CreateTunnel(c *gin.Context) {
	hash := c.Param("id")

	var req struct {
		Mode       string `json:"mode" binding:"required"`       // "pull", "push", "dynamic", "internet"
		SrcAddress string `json:"src_address"`                    // required for pull/push/internet
		DstAddress string `json:"dst_address"`                    // required for pull/push/internet
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode is required"})
		return
	}

	client := core.FindTermiteClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "termite session not found (tunnels require encrypted client)"})
		return
	}

	switch req.Mode {
	case "pull":
		core.AddPullTunnelConfig(client, req.DstAddress, req.SrcAddress)
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": "pull tunnel created"})
	case "push":
		core.AddPushTunnelConfig(client, req.SrcAddress, req.DstAddress)
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": "push tunnel created"})
	case "dynamic":
		client.StartSocks5Server()
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": "dynamic tunnel (socks5) requested"})
	case "internet":
		if _, exists := core.Ctx.Socks5Servers[req.SrcAddress]; exists {
			c.JSON(http.StatusConflict, gin.H{"error": "socks5 server already exists at " + req.SrcAddress})
			return
		}
		if err := core.StartSocks5Server(req.SrcAddress); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		core.AddPushTunnelConfig(client, req.SrcAddress, req.DstAddress)
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": "internet tunnel created"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode, use: pull, push, dynamic, internet"})
	}
}

// tunnelInfoEntry is one row of ListTunnels's response.
type tunnelInfoEntry struct {
	Type    string `json:"type"`    // "pull", "push", or "socks5"
	Address string `json:"address"` // src→dst for pull/push, single addr for socks5
}

// ListTunnels returns the active tunnels for a session.
//
// @Summary     List tunnels
// @Description List every pull/push/SOCKS5 tunnel currently routed through this session.
// @Tags        tunnels
// @Produce     json
// @Security    BearerAuth
// @Param       id  path   string  true "Session hash"
// @Success     200 {object} map[string]any "status + tunnels:[]tunnelInfoEntry"
// @Router      /api/v1/sessions/{id}/tunnels [get]
func ListTunnels(c *gin.Context) {
	type tunnelInfo struct {
		Type    string `json:"type"`
		Address string `json:"address"`
	}

	var tunnels []tunnelInfo
	for addr, tc := range core.Ctx.PullTunnelConfig {
		tunnels = append(tunnels, tunnelInfo{Type: "pull", Address: addr + " → " + tc.Address})
	}
	for addr, tc := range core.Ctx.PushTunnelConfig {
		tunnels = append(tunnels, tunnelInfo{Type: "push", Address: tc.Address + " → " + addr})
	}
	for addr := range core.Ctx.Socks5Servers {
		tunnels = append(tunnels, tunnelInfo{Type: "socks5", Address: addr})
	}

	c.JSON(http.StatusOK, gin.H{"status": true, "tunnels": tunnels})
}
