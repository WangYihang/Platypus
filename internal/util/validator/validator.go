package validator

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

func FormExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.PostForm(param) == "" {
			return PanicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func ParamsExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			return PanicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func PanicRESTfully(c *gin.Context, msg string) bool {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    msg,
	})
	c.Abort()
	return false
}
