package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core/artifact"
	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/log"
)

// latestVersionSentinel lets callers request the current channel head
// without having to know its version number. Used by the bootstrap
// install script (see renderInstallScript) so admins don't have to
// pin a version when pasting the one-liner.
const latestVersionSentinel = "latest"

// Manifest is the signed document that pins which artifact the agent
// should download for its (os, arch). It lives in the object store at
// {prefix}/manifest/{channel}.json and is signed with a detached
// Ed25519 signature at {prefix}/manifest/{channel}.json.sig.
type Manifest struct {
	Version    string             `json:"version"`
	Channel    string             `json:"channel"`
	ReleasedAt time.Time          `json:"released_at"`
	Artifacts  []ManifestArtifact `json:"artifacts"`
}

// ManifestArtifact describes a single platform build.
type ManifestArtifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Key    string `json:"key"` // object-store key, relative to the store prefix
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"` // hex-encoded
}

// Distributor is the HTTP facade in front of the release artifact store.
// It never serves binary bytes itself — it returns the signed manifest
// and 302-redirects to short-lived presigned URLs for artifact downloads.
type Distributor struct {
	Url          string         `json:"url"`
	Channel      string         `json:"channel"`
	PresignedTTL time.Duration  `json:"-"`
	Store        artifact.Store `json:"-"`
}

// RegisterDistributorRoutes mounts the distributor + installer routes
// on an existing gin engine. Since PR-E the distributor no longer owns
// its own port — it shares the unified-ingress HTTP virtual listener
// with the REST API, so this is the only entry point callers need.
//
// The Distributor instance is stashed on core.Ctx.Distributor so legacy
// callers (e.g. the update sender in agent.go) can look it up without a
// param thread.
func RegisterDistributorRoutes(engine *gin.Engine, cfg DistributorArgs) *Distributor {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard

	d := &Distributor{
		Url:          cfg.Url,
		Channel:      cfg.Channel,
		PresignedTTL: cfg.PresignedTTL,
		Store:        cfg.Store,
	}
	if Ctx != nil {
		Ctx.Distributor = d
	}

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	engine.GET("/v1/manifest/:channel", d.handleManifest)
	engine.GET("/v1/manifest/:channel/signature", d.handleManifestSignature)
	engine.GET("/v1/artifacts/:os/:arch/:version", d.handleArtifact)

	// GET /install/<dl-token> returns a runnable shell script with the
	// minted PAT hard-coded into the agent command line. Single-use:
	// the install token gets marked consumed atomically on first valid
	// hit, and subsequent curls receive 404. Public (no JWT) — the
	// bootstrap script must be reachable before the agent has any
	// credential to authenticate with.
	engine.GET("/install/:token", serveInstallScript)

	return d
}

// DistributorArgs bundles the inputs to RegisterDistributorRoutes so
// we can grow the surface without churning call sites.
type DistributorArgs struct {
	Url          string
	Channel      string
	PresignedTTL time.Duration
	Store        artifact.Store
}

// handleManifest returns the raw manifest JSON for the requested
// channel. It's small, so we forward the bytes directly rather than
// issuing a presigned URL — the agent needs the bytes anyway to verify
// the signature.
func (d *Distributor) handleManifest(c *gin.Context) {
	channel := c.Param("channel")
	if channel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel is required"})
		return
	}
	key := fmt.Sprintf(artifact.ManifestKeyFmt, channel)
	data, err := d.Store.GetObject(c.Request.Context(), key)
	if err != nil {
		log.Error("distributor: fetch manifest %s: %s", channel, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

// handleManifestSignature returns the detached Ed25519 signature that
// authenticates the manifest bytes.
func (d *Distributor) handleManifestSignature(c *gin.Context) {
	channel := c.Param("channel")
	if channel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel is required"})
		return
	}
	key := fmt.Sprintf(artifact.ManifestSigKeyFmt, channel)
	data, err := d.Store.GetObject(c.Request.Context(), key)
	if err != nil {
		log.Error("distributor: fetch manifest signature %s: %s", channel, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "signature not found"})
		return
	}
	c.Data(http.StatusOK, "application/octet-stream", data)
}

// handleArtifact resolves (os, arch, version) through the default
// channel's manifest and redirects to a presigned download URL. The
// Distributor never streams artifact bytes itself. `version` may be
// the literal "latest", in which case it resolves to whatever the
// current channel manifest pins.
func (d *Distributor) handleArtifact(c *gin.Context) {
	osName := c.Param("os")
	archName := c.Param("arch")
	version := c.Param("version")
	if osName == "" || archName == "" || version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "os, arch, version are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	m, err := d.loadManifest(ctx, d.Channel)
	if err != nil {
		log.Error("distributor: load manifest: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "manifest unavailable"})
		return
	}
	if version != latestVersionSentinel && m.Version != version {
		c.JSON(http.StatusNotFound, gin.H{
			"error":           "version not served by current channel",
			"requested":       version,
			"channel_version": m.Version,
			"channel":         d.Channel,
		})
		return
	}
	art := m.findArtifact(osName, archName)
	if art == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no artifact for %s/%s", osName, archName)})
		return
	}
	url, _, err := d.Store.PresignGet(ctx, art.Key, d.PresignedTTL)
	if err != nil {
		log.Error("distributor: presign %s: %s", art.Key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "presign failed"})
		return
	}
	c.Redirect(http.StatusFound, url)
}

// loadManifest fetches the channel's manifest and parses it. The
// signature is not verified here — the agent is the party that needs
// to trust it, so it fetches and verifies the signature independently.
func (d *Distributor) loadManifest(ctx context.Context, channel string) (*Manifest, error) {
	data, err := d.Store.GetObject(ctx, fmt.Sprintf(artifact.ManifestKeyFmt, channel))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

func (m *Manifest) findArtifact(os, arch string) *ManifestArtifact {
	for i := range m.Artifacts {
		if m.Artifacts[i].OS == os && m.Artifacts[i].Arch == arch {
			return &m.Artifacts[i]
		}
	}
	return nil
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
// agent binary from the current channel's manifest and runs it with the
// freshly-minted PAT injected via --token.
//
// The script detects GOOS/GOARCH via `uname` at run time and pulls from
// /v1/artifacts/{os}/{arch}/latest — the distributor resolves "latest"
// against the current signed manifest and 302-redirects to a short-
// lived presigned URL from the object store.
//
// Kept deliberately tiny — admins and security reviewers should be able
// to read the whole thing in one screen and verify it doesn't do
// anything surprising.
func renderInstallScript(r *enrollment.ConsumeResult, distributorHost string) string {
	endpoint := r.ServerEndpoint
	host, port := splitHostPort(endpoint)
	// The distributor base URL (scheme://host[:port]) the agent will
	// hit for the artifact. Unified ingress always terminates TLS, so
	// the generated one-liner always uses https. The `-k` on curl
	// tolerates the default self-signed cert operators typically ship
	// with; swap it out for a real cert and remove the flag by setting
	// cfg.Distributor.Url to an https URL (the install script honours
	// it if present).
	base := "https://" + shellQuoteHostInline(distributorHost)
	lines := []string{
		"#!/bin/sh",
		"# Platypus agent bootstrap — generated by the server.",
		"# This script is for one-shot enrollment; the download token is",
		"# burnt on first successful hit.",
		"set -eu",
		"AGENT_HOST=" + shellQuote(host),
		"AGENT_PORT=" + shellQuote(port),
		"AGENT_TOKEN=" + shellQuote(r.PATPlaintext),
	}
	if r.ProjectCAPEM != "" {
		// base64 so the PEM's literal newlines survive the shell
		// round trip; agent-side code will base64-decode on first
		// use (see internal/agent/trust.go in Phase II).
		lines = append(lines,
			"PLATYPUS_PROJECT_CA="+shellQuote(base64.StdEncoding.EncodeToString([]byte(r.ProjectCAPEM))),
			"export PLATYPUS_PROJECT_CA",
		)
	}
	lines = append(lines,
		"OS=$(uname -s | tr '[:upper:]' '[:lower:]')",
		"case \"$(uname -m)\" in",
		"  x86_64|amd64) ARCH=amd64 ;;",
		"  aarch64|arm64) ARCH=arm64 ;;",
		"  *) echo \"unsupported arch: $(uname -m)\" >&2; exit 1 ;;",
		"esac",
		"BIN=$(mktemp /tmp/platypus-agent-XXXXXX)",
		"curl -fsSLk "+base+"/v1/artifacts/\"$OS\"/\"$ARCH\"/latest -o \"$BIN\"",
		"chmod +x \"$BIN\"",
		"exec \"$BIN\" --host \"$AGENT_HOST\" --port \"$AGENT_PORT\" --token \"$AGENT_TOKEN\"",
	)
	return strings.Join(lines, "\n") + "\n"
}

// shellQuote returns the input wrapped in single quotes, with any
// embedded single quote escaped. Robust against the (unexpected) case
// of a malformed endpoint.
func shellQuote(s string) string {
	// Replace ' with '\'' then wrap.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellQuoteHostInline returns a Host string safe to splice into a
// shell URL literal. If the host contains shell-special characters we
// fall back to a printf %s subshell so nothing gets interpreted.
func shellQuoteHostInline(s string) string {
	if strings.ContainsAny(s, " '\"$`\n") {
		return "$(printf %s " + shellQuote(s) + ")"
	}
	return s
}

// splitHostPort divides "host:port". Returns (s, "") if malformed.
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
