package binary

import (
	"context"
	"net/http"

	binary_model "github.com/WangYihang/Platypus/internal/models/binary"
	"github.com/WangYihang/Platypus/internal/utils/compiler"
	http_util "github.com/WangYihang/Platypus/internal/utils/http"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

func GetAllBinaries(c *gin.Context) {
	ctx := context.Background()
	span := sentry.StartSpan(ctx, "binary", sentry.TransactionName("GetAllBinaries"))
	c.IndentedJSON(http.StatusOK, gin.H{
		"status":  true,
		"message": binary_model.GetAllBinaries(),
	})
	span.Finish()
}

func GetBinary(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	if binary, err := binary_model.GetBinaryByID(c.Param("id")); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status":  true,
			"message": binary,
		})
	}
}

func RawBinary(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		return
	}
	if data, err := binary_model.RawBinary(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": err.Error(),
		})
	} else {
		c.Data(http.StatusOK, "application/json", data)
	}
}

// CompileHandler godoc
// @Summary     Compile to generate a agent binary
// @Description Compile agent binary using provided os/arch info, returns the downloading partial url path of generated binary path.
// @Tags        Compiler
// @Accept      json
// @Produce     json
// @Param       os   query    string true "operating system" Enums(aix, android, darwin, dragonfly, freebsd, hurd, illumos, ios, js, linux, nacl, netbsd, openbsd, plan9, solaris, windows, zos) default(linux)
// @Param       arch query    string true "architecture"     Enums(386, amd64, arm, arm64, mips, mips64, mips64le, mipsle, ppc64, ppc64le, riscv64, s390x, wasm)                                 default(amd64)
// @Param       host query    string true "host" example("127.0.0.1", "platypus.foobar.com")
// @Param       port query    int    true "port"      minimum(0) maximum(65535)
// @Param       upx  query    int    true "upx level" Enums(0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
// @Success     200  {object} string
// @Router      /compile [post]
// @Security    ApiKeyAuth
func CreateBinary(c *gin.Context) {
	var query struct {
		Os   string `form:"os" json:"os" binding:"required,oneof=linux windows darwin"`
		Arch string `form:"arch" json:"arch" binding:"required,oneof=amd64 386 arm arm64"`
		Host string `form:"host" json:"host" binding:"required,ip|hostname"`
		Port uint16 `form:"port" json:"port" binding:"required,numeric,max=65535,min=0"`
		Upx  int    `form:"upx" json:"upx" binding:"required,numeric,max=9,min=0"`
	}
	if err := c.ShouldBind(&query); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
		return
	}
	relativePath, err := compiler.DoCompile(query.Os, query.Arch, query.Host, query.Port, query.Upx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": relativePath,
	})
}
