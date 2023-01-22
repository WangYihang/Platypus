package http

import (
	"net/http"

	"github.com/WangYihang/Platypus/internal/utils/str"
	"github.com/gin-gonic/gin"
)

func CheckIDExists(c *gin.Context) bool {
	if !str.IsValidUUID(c.Param("id")) {
		c.AbortWithStatusJSON(http.StatusOK, gin.H{
			"status":  false,
			"message": "Invalid ID",
		})
		return false
	}
	return true
}
