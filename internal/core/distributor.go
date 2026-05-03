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
	"github.com/WangYihang/Platypus/pkg/installbundle"
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

// DistributorSettings is the live policy surface the distributor
// consults on every request. The production implementation is
// settings.Registry; tests pass a stub. Kept as an interface so the
// core package has no compile-time dependency on internal/settings.
type DistributorSettings interface {
	DistributorChannel() string
	DistributorPresignedTTL() time.Duration
}

// Distributor is the HTTP facade in front of the release artifact store.
// It never serves binary bytes itself — it returns the signed manifest
// and 302-redirects to short-lived presigned URLs for artifact downloads.
type Distributor struct {
	Settings DistributorSettings `json:"-"`
	Store    artifact.Store      `json:"-"`
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
		Settings: cfg.Settings,
		Store:    cfg.Store,
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

	// GET /api/v1/install/<dl-token> returns a runnable shell script
	// with the minted PAT hard-coded into the agent command line.
	// Single-use: the install token gets marked consumed atomically on
	// first valid hit, and subsequent curls receive 404. Public (no
	// JWT) — the bootstrap script must be reachable before the agent
	// has any credential to authenticate with.
	engine.GET("/api/v1/install/:token", serveInstallScript)

	return d
}

// DistributorArgs bundles the inputs to RegisterDistributorRoutes so
// we can grow the surface without churning call sites.
type DistributorArgs struct {
	Settings DistributorSettings
	Store    artifact.Store
}

// currentChannel resolves the live release channel. The distributor
// consults the settings registry on every request so admin edits take
// effect immediately. When Settings is nil (e.g. unit tests without a
// registry) it falls back to the hardcoded default so tests stay
// trivial.
func (d *Distributor) currentChannel() string {
	if d.Settings != nil {
		if v := d.Settings.DistributorChannel(); v != "" {
			return v
		}
	}
	return "stable"
}

// currentPresignedTTL mirrors currentChannel for the presigned URL
// lifetime.
func (d *Distributor) currentPresignedTTL() time.Duration {
	if d.Settings != nil {
		if d := d.Settings.DistributorPresignedTTL(); d > 0 {
			return d
		}
	}
	return 5 * time.Minute
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

	channel := d.currentChannel()
	m, err := d.loadManifest(ctx, channel)
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
			"channel":         channel,
		})
		return
	}
	art := m.findArtifact(osName, archName)
	if art == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no artifact for %s/%s", osName, archName)})
		return
	}
	reader, size, contentType, err := d.Store.GetObjectReader(ctx, art.Key)
	if err != nil {
		log.Error("distributor: open artifact %s: %s", art.Key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "artifact unavailable"})
		return
	}
	defer reader.Close()
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.DataFromReader(http.StatusOK, size, contentType, reader, nil)
}

// LivePlatforms loads the active channel's manifest and returns the
// (os, arch) pairs it pins, alongside the channel name and version.
// A missing or unreadable manifest yields an empty Artifacts slice and
// nil error so the install dialog can render an explicit "publish first"
// hint instead of failing the request — matches the rest of the
// distributor's "best-effort, never block the UI" stance. The
// underlying error is still logged for operator visibility.
func (d *Distributor) LivePlatforms(ctx context.Context) (channel, version string, artifacts []ManifestArtifact) {
	channel = d.currentChannel()
	m, err := d.loadManifest(ctx, channel)
	if err != nil {
		log.Warn("distributor: live platforms: %s", err)
		return channel, "", nil
	}
	return channel, m.Version, m.Artifacts
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

// serveInstallScript handles GET /api/v1/install/<dl_id>.<secret>. It is the
// public, auth-free counterpart to POST /api/v1/projects/:pid/install-
// artifacts. The default response is a POSIX shell script the admin
// pastes into `curl ... | sh`; with ?format=bundle the response is a
// single-line `pinst_<base64>` token the user pastes straight into
// `platypus-agent` for offline / scripted bootstrap.
//
// Either format consumes the install token exactly once. Choosing
// the format at curl time means the admin doesn't have to mint two
// separate install tokens for the two flows.
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

	if c.Query("format") == "bundle" {
		bundle, err := installbundle.Encode(installbundle.Bundle{
			Server:            res.ServerEndpoint,
			PAT:               res.PATPlaintext,
			CACertPEM:         res.ProjectCAPEM,
			ProjectID:         res.ProjectID,
			BaselinePluginIDs: res.BaselinePluginIDs,
		})
		if err != nil {
			log.Error("distributor: encode install bundle: %s", err)
			c.String(http.StatusInternalServerError, "encode install bundle")
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, bundle)
		return
	}

	forceInsecureDownload := strings.EqualFold(c.Query("download_tls"), "insecure")
	if c.Query("os") == "windows" {
		script := renderInstallScriptPS1(res, c.Request.Host, forceInsecureDownload)
		// text/plain so the response stays inside `iwr | iex` happily;
		// PowerShell doesn't need a special MIME and treats anything
		// printable as script text.
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, script)
		return
	}

	script := renderInstallScript(res, c.Request.Host, forceInsecureDownload)
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
func renderInstallScript(r *enrollment.ConsumeResult, distributorHost string, forceInsecureDownload bool) string {
	endpoint := r.ServerEndpoint
	host, port := splitHostPort(endpoint)
	// The distributor base URL (scheme://host[:port]) the agent will
	// hit for the artifact. Unified ingress always terminates TLS.
	//
	// TLS verification policy: when the project has a CA initialised the
	// server stamps it into the script via PLATYPUS_PROJECT_CA and the
	// downloader pins on it (--cacert). When no CA is available we refuse
	// to download by default — the operator must opt in explicitly with
	// PLATYPUS_INSECURE_DOWNLOAD=1, with a loud warning. This prevents a
	// network attacker from replacing the agent binary on a default
	// install (CVE-class: MITM → remote code execution as root).
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
		// Comma-separated list rendered into --baseline-plugins below.
		// Empty string is the safe default: the agent boots with only
		// its mandatory core plugin (sys-info) and operators add
		// other capabilities later via the per-agent plugin REST.
		"AGENT_BASELINE_PLUGINS=" + shellQuote(strings.Join(r.BaselinePluginIDs, ",")),
	}
	if r.ProjectCAPEM != "" {
		// base64 so the PEM's literal newlines survive the shell
		// round trip; the downloader below decodes this into a
		// temp file fed to curl --cacert.
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
		"CA_FILE=\"\"",
		"trap 'if [ -n \"$CA_FILE\" ] && [ -f \"$CA_FILE\" ]; then rm -f \"$CA_FILE\"; fi' EXIT",
		// TLS_MODE is the per-tool trust mode the download function
		// consults. "ca" → use $CA_FILE; "insecure" → skip
		// verification. We compute it once here so each downloader
		// branch in download_with_fallback stays branch-light.
		"TLS_MODE=\"\"",
	)
	if forceInsecureDownload {
		lines = append(lines,
			"echo 'warning: insecure download mode forced by install command selection; skipping TLS verification on agent download' >&2",
			"TLS_MODE=insecure",
		)
	} else {
		lines = append(lines,
			"if [ -n \"${PLATYPUS_PROJECT_CA-}\" ]; then",
			"  CA_FILE=$(mktemp /tmp/platypus-ca-XXXXXX.pem)",
			"  printf '%s' \"$PLATYPUS_PROJECT_CA\" | base64 -d > \"$CA_FILE\"",
			"  TLS_MODE=ca",
			"elif [ \"${PLATYPUS_INSECURE_DOWNLOAD-0}"+"\" = \"1\" ]; then",
			"  echo 'warning: PLATYPUS_INSECURE_DOWNLOAD=1, skipping TLS verification on agent download' >&2",
			"  TLS_MODE=insecure",
			"else",
			"  echo 'platypus: server has no project CA in this install script and PLATYPUS_INSECURE_DOWNLOAD is not set' >&2",
			"  echo 'platypus: refusing to download agent binary without TLS trust anchor (MITM risk)' >&2",
			"  echo 'platypus: ask the server admin to initialise a project CA, or re-run with PLATYPUS_INSECURE_DOWNLOAD=1 if you accept the risk' >&2",
			"  exit 1",
			"fi",
		)
	}
	lines = append(lines,
		// download_with_fallback fetches $1 → $2 by trying curl, then
		// wget, then python3, then fetch (BSD), in that order. Each
		// branch honours $TLS_MODE. Returns 0 on first success, 1 if
		// every tool either is missing or failed. Keeping the cascade
		// inline (instead of a helper script) preserves the
		// "auditable in one screen" property of the install payload.
		//
		// Why a cascade is necessary even after the outer one-liner
		// works: macOS ships curl linked against an old LibreSSL that
		// throws "unsupported algorithm" against some self-signed
		// server certs. The outer command may have used wget/python3
		// to get past that, but the inner fetch of the agent binary
		// hits the same broken curl unless we fall through.
		"download_with_fallback() {",
		"  _url=$1",
		"  _out=$2",
		"  if command -v curl >/dev/null 2>&1; then",
		"    _flag=\"-k\"",
		"    [ \"$TLS_MODE\" = ca ] && _flag=\"--cacert $CA_FILE\"",
		"    if curl -fsSL --tlsv1.2 $_flag \"$_url\" -o \"$_out\" 2>/dev/null; then return 0; fi",
		"    echo 'platypus: curl failed, trying wget...' >&2",
		"  fi",
		"  if command -v wget >/dev/null 2>&1; then",
		"    _flag=\"--no-check-certificate\"",
		"    [ \"$TLS_MODE\" = ca ] && _flag=\"--ca-certificate=$CA_FILE\"",
		"    if wget -q $_flag \"$_url\" -O \"$_out\" 2>/dev/null; then return 0; fi",
		"    echo 'platypus: wget failed, trying python3...' >&2",
		"  fi",
		"  if command -v python3 >/dev/null 2>&1; then",
		"    if PLATYPUS_URL=\"$_url\" PLATYPUS_OUT=\"$_out\" PLATYPUS_TLS=\"$TLS_MODE\" PLATYPUS_CA=\"$CA_FILE\" python3 -c '\nimport os, ssl, sys, urllib.request\nmode=os.environ.get(\"PLATYPUS_TLS\",\"\")\nif mode==\"ca\":\n    ctx=ssl.create_default_context(cafile=os.environ[\"PLATYPUS_CA\"])\nelse:\n    ctx=ssl._create_unverified_context()\nwith urllib.request.urlopen(os.environ[\"PLATYPUS_URL\"], context=ctx) as r, open(os.environ[\"PLATYPUS_OUT\"],\"wb\") as f:\n    while True:\n        b=r.read(65536)\n        if not b: break\n        f.write(b)\n' 2>/dev/null; then return 0; fi",
		"    echo 'platypus: python3 failed, trying fetch...' >&2",
		"  fi",
		"  if command -v fetch >/dev/null 2>&1; then",
		"    _flag=\"--no-verify-peer\"",
		"    [ \"$TLS_MODE\" = ca ] && _flag=\"--ca-cert=$CA_FILE\"",
		"    if fetch -q $_flag -o \"$_out\" \"$_url\" 2>/dev/null; then return 0; fi",
		"  fi",
		"  echo 'platypus: no working downloader (curl/wget/python3/fetch all missing or failed)' >&2",
		"  return 1",
		"}",
		"BIN=$(mktemp /tmp/platypus-agent-XXXXXX)",
		"download_with_fallback "+base+"/v1/artifacts/\"$OS\"/\"$ARCH\"/latest \"$BIN\"",
		"chmod +x \"$BIN\"",
		// Single --server flag carries host:port, token stays positional.
		// --baseline-plugins is always rendered (even when empty) so
		// the agent's first-boot logs show the operator decision
		// explicitly rather than relying on the absence of a flag.
		"exec \"$BIN\" --server \"$AGENT_HOST:$AGENT_PORT\" --baseline-plugins \"$AGENT_BASELINE_PLUGINS\" \"$AGENT_TOKEN\"",
	)
	return strings.Join(lines, "\n") + "\n"
}

// renderInstallScriptPS1 builds the PowerShell counterpart to
// renderInstallScript for Windows targets. Same shape as the POSIX
// script: download the agent for the detected arch from the current
// channel's manifest (pinned to the project CA when available), then
// exec it with the freshly-minted PAT.
//
// Reachable only via `?os=windows` on the install endpoint — the
// admin-facing one-liner forces that query string when the wizard's
// OS picker is `windows`. Stock Windows ships with PowerShell but not
// curl, so the unix script wouldn't run there.
//
// Kept short for the same reason the unix script is: an operator (or
// SOC reviewer) should be able to read the whole thing in one screen
// and verify it does nothing surprising.
func renderInstallScriptPS1(r *enrollment.ConsumeResult, distributorHost string, forceInsecureDownload bool) string {
	endpoint := r.ServerEndpoint
	host, port := splitHostPort(endpoint)
	base := "https://" + distributorHost
	lines := []string{
		"# Platypus agent bootstrap (Windows / PowerShell).",
		"# One-shot enrollment; the download token is burnt on first hit.",
		"$ErrorActionPreference = 'Stop'",
		// Force TLS 1.2 on the .NET ServicePointManager — Windows
		// PowerShell 5.1 defaults to Ssl3|Tls (TLS 1.0), which the
		// ingress listener (MinVersion=TLS 1.2) rejects with the
		// misleading "Could not create SSL/TLS secure channel"
		// error. Setting this here is defensive: the outer iwr|iex
		// one-liner already forces it, but a paste of the bare
		// script body via a different harness would otherwise miss
		// the override and fail.
		"[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12",
		"$AgentHost = " + psQuote(host),
		"$AgentPort = " + psQuote(port),
		"$AgentToken = " + psQuote(r.PATPlaintext),
		"$AgentBaselinePlugins = " + psQuote(strings.Join(r.BaselinePluginIDs, ",")),
		"$DistBase  = " + psQuote(base),
	}
	if forceInsecureDownload {
		lines = append(lines,
			"Write-Warning 'insecure download mode forced by install command selection; skipping TLS verification on agent download'",
			"[System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }",
		)
	} else if r.ProjectCAPEM != "" {
		// CA pinning in PowerShell: install a per-process server-cert
		// validator that accepts a chain only when it terminates at
		// the project CA. Mirrors the unix script's `curl --cacert`.
		lines = append(lines,
			"$CaB64 = "+psQuote(base64.StdEncoding.EncodeToString([]byte(r.ProjectCAPEM))),
			"$Ca = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2(,[Convert]::FromBase64String($CaB64))",
			"[System.Net.ServicePointManager]::ServerCertificateValidationCallback = {",
			"  param($s, $cert, $chain, $err)",
			"  $c = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($cert)",
			"  $ch = New-Object System.Security.Cryptography.X509Certificates.X509Chain",
			"  $ch.ChainPolicy.ExtraStore.Add($Ca) | Out-Null",
			"  $ch.ChainPolicy.RevocationMode = 'NoCheck'",
			"  $ch.ChainPolicy.VerificationFlags = 'AllowUnknownCertificateAuthority'",
			"  if (-not $ch.Build($c)) { return $false }",
			"  foreach ($e in $ch.ChainElements) {",
			"    if ($e.Certificate.Thumbprint -eq $Ca.Thumbprint) { return $true }",
			"  }",
			"  return $false",
			"}",
		)
	} else {
		lines = append(lines,
			"if ($env:PLATYPUS_INSECURE_DOWNLOAD -eq '1') {",
			"  Write-Warning 'PLATYPUS_INSECURE_DOWNLOAD=1, skipping TLS verification on agent download'",
			"  [System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }",
			"} else {",
			"  Write-Error 'platypus: server has no project CA in this install script and PLATYPUS_INSECURE_DOWNLOAD is not set. Refusing to download agent binary without TLS trust anchor (MITM risk). Ask the server admin to initialise a project CA, or re-run with $env:PLATYPUS_INSECURE_DOWNLOAD=1 if you accept the risk.'",
			"  exit 1",
			"}",
		)
	}
	lines = append(lines,
		"switch ($env:PROCESSOR_ARCHITECTURE) {",
		"  'AMD64' { $Arch = 'amd64' }",
		"  'ARM64' { $Arch = 'arm64' }",
		"  'x86'   { $Arch = '386' }",
		"  default { Write-Error \"unsupported arch: $($env:PROCESSOR_ARCHITECTURE)\"; exit 1 }",
		"}",
		"$Bin = Join-Path $env:TEMP (\"platypus-agent-\" + [Guid]::NewGuid().ToString('N') + \".exe\")",
		"$Url = \"$DistBase/v1/artifacts/windows/$Arch/latest\"",
		// Try Invoke-WebRequest first (works on every supported PS
		// version), fall through to Start-BitsTransfer (built-in
		// since Windows 7) when IWR fails — useful when corporate
		// proxies block IWR but allow BITS, or when older PS chokes
		// on a TLS combination the CertificateValidationCallback
		// override didn't appease.
		"$DownloadOk = $false",
		"try {",
		"  Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $Bin",
		"  $DownloadOk = $true",
		"} catch {",
	)
	if forceInsecureDownload {
		// Start-BitsTransfer goes through BITS, which uses Windows'
		// system trust store and IGNORES the per-process SPM
		// callback. In force-insecure mode the operator has
		// explicitly opted out of cert verification — falling
		// through to BITS would replace one misleading error with
		// another ("certificate authority is invalid or incorrect"
		// from BITS even though IWR's `{ $true }` accepts the cert).
		// Surface the IWR failure instead.
		lines = append(lines,
			"  Write-Error \"Invoke-WebRequest failed in force-insecure mode: $($_.Exception.Message)\"; exit 1",
		)
	} else {
		// Cert-pinning mode: IWR honours the SPM callback so the
		// only reasons it fails here are network-level. BITS is a
		// useful escape hatch on locked-down hosts that block IWR
		// but allow background transfers.
		lines = append(lines,
			"  Write-Warning \"Invoke-WebRequest failed: $($_.Exception.Message). Trying Start-BitsTransfer...\"",
			"  try { Start-BitsTransfer -Source $Url -Destination $Bin -ErrorAction Stop; $DownloadOk = $true }",
			"  catch { Write-Error \"Start-BitsTransfer also failed: $($_.Exception.Message)\"; exit 1 }",
		)
	}
	lines = append(lines,
		"}",
		"if (-not $DownloadOk) { Write-Error 'platypus: no working downloader on this Windows host'; exit 1 }",
		// Single --server flag carries host:port, token stays positional.
		// --baseline-plugins is always rendered so the operator-chosen
		// system-plugin allowlist is part of the audited command line.
		"& $Bin --server \"${AgentHost}:${AgentPort}\" --baseline-plugins $AgentBaselinePlugins $AgentToken",
	)
	return strings.Join(lines, "\r\n") + "\r\n"
}

// psQuote wraps a string for safe interpolation inside a PowerShell
// single-quoted literal. Inside single quotes PowerShell only treats
// `'` specially (escaped by doubling), so this is sufficient for the
// values we splice (host, port, token, base URL).
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
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
