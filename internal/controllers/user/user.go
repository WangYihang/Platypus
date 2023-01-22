package user

import (
	"net/http"

	user_model "github.com/WangYihang/Platypus/internal/models/user"
	"github.com/gin-gonic/gin"
)

func GetUsers(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, user_model.GetAllUsers())
}

func GetCurrentUser(c *gin.Context) {
	user := user_model.User{}
	c.IndentedJSON(http.StatusOK, user)
}
