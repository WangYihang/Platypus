package Middlewares

import (
	"github.com/WangYihang/Platypus/internal/context/Models"
	"github.com/gin-gonic/gin"
	"net/http"
)

func Super() gin.HandlerFunc {
	return SuperVerify
}

func SuperVerify(c *gin.Context) {
	username, ok := c.Get(CtxUser)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "您无权限访问该页面",
		})
		c.Abort()
		return
	}
	var user Models.User
	Models.Db.Preload("Roles").First(&user, "user_name = ?", username)
	for _, role := range user.Roles {
		if role.Grade == Models.SuperRole {
			c.Next()
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": false,
		"msg":    "您无权限访问该页面",
	})
	c.Abort()
}
