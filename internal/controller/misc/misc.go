package misc

import (
	"github.com/WangYihang/Platypus/internal/util/compiler"
	"github.com/gin-gonic/gin"
)

func CompileHandler(c *gin.Context) {
	var query struct {
		Os   string `form:"os" json:"os" binding:"required,oneof=linux windows darwin"`
		Arch string `form:"arch" json:"arch" binding:"required,oneof=amd64 386 arm arm64"`
		Host string `form:"host" json:"host" binding:"required,ip|hostname"`
		Port uint16 `form:"port" json:"port" binding:"required,numeric,max=65535,min=0"`
	}
	if err := c.ShouldBind(&query); err != nil {
		c.JSON(200, gin.H{
			"status": false,
			"msg":    err.Error(),
		})
		return
	}
	relativePath, err := compiler.DoCompile(query.Os, query.Arch, query.Host, query.Port)
	if err != nil {
		c.JSON(200, gin.H{
			"status": false,
			"msg":    err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"status": true,
		"msg":    relativePath,
	})
}
