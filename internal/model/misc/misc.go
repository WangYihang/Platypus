package misc

import (
	"github.com/WangYihang/Platypus/internal/util/update"
	"github.com/gin-gonic/gin"
)

func GetCpuUsage(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func GetMemoryUsage(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func GetGoRoutineUsage(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func GetVersion(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": true,
		"msg":    update.Version,
	})
}
