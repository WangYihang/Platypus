package template

import (
	"context"
	"net/http"

	template_model "github.com/WangYihang/Platypus/internal/models/template"
	http_util "github.com/WangYihang/Platypus/internal/utils/http"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

func GetAllTemplates(c *gin.Context) {
	ctx := context.Background()
	span := sentry.StartSpan(ctx, "template", sentry.TransactionName("GetAllTemplates"))
	c.IndentedJSON(http.StatusOK, gin.H{
		"status":  true,
		"message": template_model.GetAllTemplates(),
	})
	span.Finish()
}

func GetTemplate(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	if template, err := template_model.GetTemplateByID(c.Param("id")); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status":  true,
			"message": template,
		})
	}
}

// func CreateTemplate(c *gin.Context) {
// 	var query struct {
// 		Os   string `form:"os" json:"os" binding:"required,oneof=linux windows darwin"`
// 		Arch string `form:"arch" json:"arch" binding:"required,oneof=amd64 386 arm arm64"`
// 	}
// 	if err := c.ShouldBind(&query); err != nil {
// 		c.JSON(http.StatusOK, gin.H{
// 			"status":  false,
// 			"message": err.Error(),
// 		})
// 		return
// 	}
// 	c.JSON(http.StatusOK, gin.H{
// 		"status":  true,
// 		"message": query,
// 	})
// }
