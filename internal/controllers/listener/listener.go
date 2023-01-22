package server

import (
	"context"
	"net/http"

	listener_model "github.com/WangYihang/Platypus/internal/models/listener"
	http_util "github.com/WangYihang/Platypus/internal/utils/http"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

func GetAllListeners(c *gin.Context) {
	ctx := context.Background()
	span := sentry.StartSpan(ctx, "listener", sentry.TransactionName("GetAllListeners"))
	c.IndentedJSON(http.StatusOK, gin.H{
		"status":  true,
		"message": listener_model.GetAllListeners(),
	})
	span.Finish()
}

func GetListener(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	if listener, err := listener_model.GetListenerByID(c.Param("id")); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status":  true,
			"message": listener,
		})
	}
}

func CreateListener(c *gin.Context) {
	var query struct {
		Host     string `form:"host" json:"host" binding:"required,ip|hostname"`
		Port     uint16 `form:"port" json:"port" binding:"required,numeric,max=65535,min=0"`
		Protocol string `form:"protocol" json:"protocol" binding:"required,oneof=plain_tcp plain_udp termite_tcp termite_udp termite_dns termite_icmp meterpreter cobaltstrike"`
		Enable   bool   `form:"enable,default=false" json:"enable" binding:"required,boolean"`
	}
	if err := c.ShouldBind(&query); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
		return
	}
	l, err := listener_model.CreateListener(query.Host, query.Port, query.Protocol, query.Enable)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": l,
	})
}

func EnableListener(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	err := listener_model.EnableListener(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "enabled",
	})
}

func DisableListener(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	err := listener_model.DisableListener(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "disabled",
	})
}

func GetAllAgentsOfListener(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func DeleteListener(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}
