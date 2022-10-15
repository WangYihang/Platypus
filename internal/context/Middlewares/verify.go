package Middlewares

import (
	"github.com/WangYihang/Platypus/internal/context/Conf"
	"github.com/gin-gonic/gin"
	"net/http"
)

const (
	CtxUser = "ctxUser"
)

// AccessVerify 先检查是否包含access code, 再检查refresh code,
func AccessVerify(c *gin.Context) {
	token, err := c.Cookie("access")
	if err == nil {
		if userName, ok := verifyAccessToken(token); ok {
			//var user Models.User
			//Models.Db.First(&user, "user_name = ?", userName)
			//if user.ID != 0 {
			// 这个分支是成功验证了access
			c.Set(CtxUser, userName)
			c.Next()
			return
			//}
		}

	}
	refreshVerify(c)

}

func refreshVerify(c *gin.Context) {
	token, err := c.Cookie("refresh")
	if err == nil {
		// 找到了refresh code//token, 验证一下, 成功的话那就设置一下用户名
		if userName, ok := verifyRefreshToken(token); ok {
			//var user Models.User
			//Models.Db.First(&user, "user_name = ?", userName)
			//if user.ID != 0 {
			c.Set(CtxUser, userName)
			ac, err := CreateAccessToken(userName)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"msg": err.Error()})
			}
			c.SetCookie("access", ac, Conf.RestfulConf.AccessExpireTime, "/", Conf.RestfulConf.Domain, false, true)
			c.Next()
			return
			//}
		}

	}
	c.JSON(http.StatusOK, gin.H{
		"status": false,
		"msg":    "用户未登录",
	})

	c.Abort()
}

// 先检查是否包含access code, 再检查refresh code,

func Oauth() gin.HandlerFunc {
	return AccessVerify
}
