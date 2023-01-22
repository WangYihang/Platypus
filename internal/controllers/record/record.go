package record

import (
	"context"
	"net/http"

	record_model "github.com/WangYihang/Platypus/internal/models/record"
	http_util "github.com/WangYihang/Platypus/internal/utils/http"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

func RawRecord(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	if data, err := record_model.RawRecord(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": err.Error(),
		})
	} else {
		c.Data(http.StatusOK, "application/json", data)
	}
}

func GetAllRecords(c *gin.Context) {
	ctx := context.Background()
	span := sentry.StartSpan(ctx, "record", sentry.TransactionName("GetAllRecords"))
	c.IndentedJSON(http.StatusOK, gin.H{
		"status":  true,
		"message": record_model.GetAllRecords(),
	})
	span.Finish()
}

func GetRecord(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	if record, err := record_model.GetRecordByID(c.Param("id")); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status":  true,
			"message": record,
		})
	}
}
