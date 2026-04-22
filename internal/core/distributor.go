package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/compiler"
	"github.com/WangYihang/Platypus/internal/utils/network"
)

func distributorParamsExist(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			c.JSON(200, gin.H{"status": false, "msg": fmt.Sprintf("%s is required", param)})
			c.Abort()
			return false
		}
	}
	return true
}

func distributorPanic(c *gin.Context, msg string) {
	c.JSON(200, gin.H{"status": false, "msg": msg})
	c.Abort()
}

type Distributor struct {
	Host       string            `json:"host"`
	Port       uint16            `json:"port"`
	Interfaces []string          `json:"interfaces"`
	Route      map[string]string `json:"route"`
	Url        string            `json:"url"`
}

// CreateDistributorServer returns a gin engine that serves on-demand agent
// binaries built for the requested connect-back target. Admins download the
// agent from here to install on a managed host.
func CreateDistributorServer(host string, port uint16, url string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	endpoint := gin.Default()

	// Connect with context
	Ctx.Distributor = &Distributor{
		Host:       host,
		Port:       port,
		Interfaces: network.GatherInterfacesList(host),
		Route:      map[string]string{},
		Url:        url,
	}

	endpoint.GET("/agent/:target", func(c *gin.Context) {
		if !distributorParamsExist(c, []string{"target"}) {
			return
		}
		target := c.Param("target")

		if target == "" {
			log.Error("Invalid connect back addr: %v", target)
			distributorPanic(c, "Invalid connect back addr")
			return
		}

		dir, filename, err := compiler.GenerateDirFilename()
		if err != nil {
			log.Error("%s", err)
			distributorPanic(c, err.Error())
			return
		}
		defer os.RemoveAll(dir)

		err = compiler.BuildAgentFromPrebuildAssets(filename, target)
		if err != nil {
			log.Error("%s", err)
			distributorPanic(c, err.Error())
			return
		}

		if !compiler.Compress(filename) {
			log.Error("Can not compress agent binary")
		}

		c.File(filename)
	})

	// GET /install/<dl-token> returns a runnable shell script with the
	// minted PAT hard-coded into the agent command line. Single-use:
	// the install token gets marked consumed atomically on first valid
	// hit, and subsequent curls receive 404. Exposed on the distributor
	// (not the REST API) because the /api/v1 surface is gated behind
	// JWT — the bootstrap script must be reachable without one.
	endpoint.GET("/install/:token", serveInstallScript)

	return endpoint
}

// serveInstallScript handles GET /install/<dl_id>.<secret>. It is the
// public, auth-free counterpart to POST /api/v1/projects/:pid/install-
// artifacts. On success the response body is a POSIX shell script the
// admin pastes into `curl ... | sh`.
func serveInstallScript(c *gin.Context) {
	if enrollSvc == nil {
		c.String(http.StatusServiceUnavailable, "server misconfigured: enrollment not enabled")
		return
	}
	token := c.Param("token")
	if token == "" {
		c.String(http.StatusBadRequest, "missing install token")
		return
	}

	res, err := enrollSvc.ConsumeInstallDownload(c.Request.Context(), token, enrollment.ConsumeContext{
		ClientIP: c.ClientIP(),
		ClientUA: c.Request.UserAgent(),
	})
	if err != nil {
		// Malformed / internal errors. Don't leak specifics to the
		// caller; the audit row has everything operators need.
		c.String(http.StatusBadRequest, "invalid install token")
		return
	}
	switch res.Outcome {
	case "success":
		// fall through
	case "unknown_id", "invalid_secret":
		c.String(http.StatusNotFound, "install token not found")
		return
	case "expired":
		c.String(http.StatusGone, "install token expired")
		return
	case "revoked":
		c.String(http.StatusGone, "install token revoked")
		return
	case "already_consumed":
		c.String(http.StatusGone, "install token already used")
		return
	default:
		c.String(http.StatusInternalServerError, "install failed")
		return
	}

	script := renderInstallScript(res, c.Request.Host)
	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.String(http.StatusOK, script)
}

// renderInstallScript builds the POSIX shell script that downloads the
// agent binary (from /agent/<target>) and runs it with the freshly-
// minted PAT injected via --token.
//
// Kept deliberately tiny — admins and security reviewers should be able
// to read the whole thing in one screen and verify it doesn't do
// anything surprising.
func renderInstallScript(r *enrollment.ConsumeResult, distributorHost string) string {
	endpoint := r.ServerEndpoint
	host, port := splitHostPort(endpoint)
	// Shell single-quoted literals are safe because our tokens and
	// endpoints are all alphanumeric / dot / underscore / colon.
	return strings.Join([]string{
		"#!/bin/sh",
		"# Platypus agent bootstrap — generated by the server.",
		"# This script is for one-shot enrollment; the download token is",
		"# burnt on first successful hit.",
		"set -eu",
		"AGENT_HOST=" + shellQuote(host),
		"AGENT_PORT=" + shellQuote(port),
		"AGENT_TOKEN=" + shellQuote(r.PATPlaintext),
		"BIN=$(mktemp /tmp/platypus-agent-XXXXXX)",
		"curl -fsSL http://" + shellQuoteHost(distributorHost) + "/agent/" + shellQuote(endpoint) + " -o \"$BIN\"",
		"chmod +x \"$BIN\"",
		"exec \"$BIN\" --host \"$AGENT_HOST\" --port \"$AGENT_PORT\" --token \"$AGENT_TOKEN\"",
	}, "\n") + "\n"
}

// shellQuote returns the input wrapped in single quotes, with any
// embedded single quote escaped. Robust against the (unexpected) case
// of a malformed endpoint.
func shellQuote(s string) string {
	// Replace ' with '\'' then wrap.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellQuoteHost is like shellQuote but returns a bare string (no quotes)
// because Host headers shouldn't contain shell-special characters. If
// they somehow do, we fall back to the quoted form for safety.
func shellQuoteHost(s string) string {
	if strings.ContainsAny(s, " '\"$`\n") {
		return "$(printf %s " + shellQuote(s) + ")"
	}
	return s
}

// splitHostPort divides "host:port". Returns ("", "") if malformed.
// We avoid net.SplitHostPort because it errors on IPv6 addresses the
// distributor sometimes sees; this shell script doesn't care about v6
// edge cases for P2.
func splitHostPort(endpoint string) (string, string) {
	idx := strings.LastIndexByte(endpoint, ':')
	if idx <= 0 || idx == len(endpoint)-1 {
		return endpoint, ""
	}
	return endpoint[:idx], endpoint[idx+1:]
}

// Quiet the unused-import checker for files that only reference context
// via other call sites — keeps the imports stable when we swap rendering
// strategies later.
var _ = context.Background
