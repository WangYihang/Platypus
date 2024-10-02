package controllers

import (
	"time"

	"github.com/WangYihang/Platypus/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// NewStatusController returns a new status controller
// Status controller returns the status of the server, including hostname, CPU usage, disk usage, and memory usage
func NewStatusController() func(c *gin.Context) {
	cache := expirable.NewLRU[string, models.Status](1, nil, time.Second*60)
	return func(c *gin.Context) {
		r, ok := cache.Get("status")
		if ok {
			c.JSON(200, r)
		} else {
			s := models.NewStatusGrabber().WithAll().Grab()
			c.JSON(200, s)
			cache.Add("status", s)
		}
	}
}
