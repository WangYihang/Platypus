package websocket

import (
	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

func CreateWebSocketServer() *melody.Melody {
	notifyWebSocket := melody.New()
	notifyWebSocket.HandleConnect(func(s *melody.Session) {
		log.Info("User (%s) connected", s.Request.RemoteAddr)
	})
	notifyWebSocket.HandleMessage(func(s *melody.Session, msg []byte) {
		log.Info("User (%s): %s", s.Request.RemoteAddr, msg)
	})
	notifyWebSocket.HandleDisconnect(func(s *melody.Session) {
		log.Info("User (%s) disconnected", s.Request.RemoteAddr)
	})
	return notifyWebSocket
}

func Notify(c *gin.Context) {
	context.Ctx.NotifyWebSocket = CreateWebSocketServer()
	context.Ctx.NotifyWebSocket.HandleRequest(c.Writer, c.Request)
}
