package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/utils/raas"
)

// ListRaasLanguages returns every template language the server can render.
//
// @Summary     List RaaS languages
// @Description Returns the sorted list of one-liner languages the server can render. These map 1:1 with internal/utils/raas/templates/*.tpl.
// @Tags        raas
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} raasLanguagesResponse
// @Router      /api/v1/raas/languages [get]
func ListRaasLanguages(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    true,
		"languages": raas.Languages(),
	})
}

// RenderRaasOneliner renders a single one-liner for a host:port+language.
//
// @Summary     Render RaaS one-liner
// @Description Returns the shell command victims should execute to connect back. Unknown languages fall back to bash so this endpoint never 404s on lang.
// @Tags        raas
// @Produce     json
// @Security    BearerAuth
// @Param       host path     string  true  "Target host (listener public IP or bind)"
// @Param       port path     integer true  "Target port 1-65535"
// @Param       lang query    string  false "Language key (bash|python|ruby|...); defaults to bash" default(bash)
// @Success     200  {object} raasOnelinerResponse
// @Failure     400  {object} errorResponse
// @Router      /api/v1/raas/oneliner [get]
func RenderRaasOneliner(c *gin.Context) {
	host := c.Query("host")
	portStr := c.Query("port")
	lang := c.DefaultQuery("lang", "bash")

	if host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host query param required"})
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "port must be 1-65535"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"oneliner": raas.Render(host, uint16(port), lang),
	})
}
