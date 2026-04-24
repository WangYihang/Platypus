package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
)

// RegisterWebSocketRoutes wires up the /notify fan-out channel used
// for one-way topology / event push from server to browsers.
//
// The v1 terminal WebSocket (/ws/:hash) was removed as part of the
// v2 migration; interactive terminals now flow through
// /api/v1/terminal/:agent_id/ws (see handler_terminal_v2.go) which
// bridges xterm.js to STREAM_TYPE_PROCESS_OPEN on the agent's v2
// yamux session.
func RegisterWebSocketRoutes(engine *gin.Engine, auth *Auth) {
	notify := newNotifyWebSocket()
	engine.GET("/notify", wsAuthMiddleware(auth), func(c *gin.Context) {
		notify.HandleRequest(c.Writer, c.Request)
	})
	core.Ctx.NotifyWebSocket = notify
}

// wsAuthMiddleware gates a WebSocket route on either a Bearer header
// OR a valid one-shot ticket in the ?ticket= query. Both paths 401 on
// failure so a WS upgrade is never attempted for unauthenticated
// clients.
func wsAuthMiddleware(auth *Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Bearer header path (native clients that can set headers).
		if h := c.GetHeader("Authorization"); h != "" {
			if parts := strings.SplitN(h, " ", 2); len(parts) == 2 &&
				strings.EqualFold(parts[0], "bearer") &&
				auth.ValidateToken(parts[1]) {
				c.Next()
				return
			}
		}
		// Ticket path (browsers).
		if tk := c.Query("ticket"); tk != "" && auth.ConsumeWSTicket(tk) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(401, gin.H{"error": "websocket auth required (Bearer header or ?ticket=)"})
	}
}

func newNotifyWebSocket() *melody.Melody {
	m := melody.New()
	// Events are small JSON blobs, but pin the cap explicitly so we
	// don't silently sit on melody's 512-byte default. Ping/pong
	// defaults (54 s / 60 s) are kept so dead connections behind
	// firewalls get reaped.
	m.Config.MaxMessageSize = 64 * 1024
	m.HandleConnect(func(s *melody.Session) {
		log.Info("Notify client connected from: %s", s.Request.RemoteAddr)
	})
	m.HandleMessage(func(s *melody.Session, msg []byte) {
		// no-op: notify is one-way (server → client)
	})
	m.HandleDisconnect(func(s *melody.Session) {
		log.Info("Notify client disconnected from: %s", s.Request.RemoteAddr)
	})
	return m
}
