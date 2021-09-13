package websocket

import (
	"github.com/WangYihang/Platypus/internal/util/log"
	"gopkg.in/olahol/melody.v1"
)

func CreateWebSocketServer() *melody.Melody {
	// Notify client online event
	notifyWebSocket := melody.New()

	notifyWebSocket.HandleConnect(func(s *melody.Session) {
		log.Info("Notify client conencted from: %s", s.Request.RemoteAddr)
	})

	notifyWebSocket.HandleMessage(func(s *melody.Session, msg []byte) {
		// Nothing to do
		log.Info("message: %s", msg)
	})

	notifyWebSocket.HandleDisconnect(func(s *melody.Session) {
		log.Info("Notify client disconencted from: %s", s.Request.RemoteAddr)
	})
	return notifyWebSocket
}
