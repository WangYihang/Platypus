package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// InstallTokensHandler serves the admin side of the "one-shot download
// link → embedded PAT" bootstrap flow. The public counterpart lives on
// the Distributor (core.CreateDistributorServer); this handler only
// mints, lists, and revokes.
type InstallTokensHandler struct {
	db     *storage.DB
	enroll *enrollment.Service

	// defaultDistributorBase is what we prepend to "/api/v1/install/<id>" when
	// composing the curl command we hand back to admins. Exposed as a
	// field so server bootstrap can inject something like
	// "http://1.2.3.4:13339" derived from distributor config.
	defaultDistributorBase string
}

func NewInstallTokensHandler(db *storage.DB, enroll *enrollment.Service, distributorBase string) *InstallTokensHandler {
	return &InstallTokensHandler{
		db:                     db,
		enroll:                 enroll,
		defaultDistributorBase: distributorBase,
	}
}

// --- Request / Response shapes -------------------------------------------

type issueInstallRequest struct {
	ServerEndpoint      string `json:"server_endpoint" binding:"required"`
	TargetOS            string `json:"target_os"`
	TargetArch          string `json:"target_arch"`
	TTLSeconds          int    `json:"ttl_seconds"`
	PATTTLSeconds       int    `json:"pat_ttl_seconds"`
	PATMaxUses          int    `json:"pat_max_uses"`
	PATBindingMachineID string `json:"pat_binding_machine_id"`
	PATDescription      string `json:"pat_description"`
	// BaselinePluginIDs is the operator-chosen list of marketplace
	// plugin ids the agent should auto-install on first boot. Empty
	// = the agent boots with no host capabilities (the secure
	// default the enroll wizard ships). The agent-side consumption
	// of this list (read on first connect, marketplace-install each
	// id) is wired in a follow-up commit; for now the value is
	// accepted + persisted on the install token's metadata so the
	// wire shape is stable while the agent catches up.
	BaselinePluginIDs []string `json:"baseline_plugin_ids"`
	// AutoApprove pre-authorizes the host that redeems this install
	// link — the host enrolls straight to `approved` without a
	// human-in-the-loop step. Used for automation flows. Default
	// false so the safer behaviour (admin reviews) wins by default.
	AutoApprove bool `json:"auto_approve"`
}

// issueInstallResponse is the only place the plaintext download token
// appears in the API. The `install_command` field is a convenience:
// a ready-to-paste one-liner admins can drop into chat / terminal.
type issueInstallResponse struct {
	DownloadID     string    `json:"download_id"`
	DownloadToken  string    `json:"download_token"` // dl_<id>.<secret>
	ExpiresAt      time.Time `json:"expires_at"`
	ServerEndpoint string    `json:"server_endpoint"`
	TargetOS       string    `json:"target_os,omitempty"`
	TargetArch     string    `json:"target_arch,omitempty"`
	// InstallCommand is the OS-default downloader's one-liner in the
	// "skip TLS verification" flavour — kept for older FE / API
	// consumers that don't know about the per-flavour maps below.
	InstallCommand string `json:"install_command"`
	// InstallCommands is keyed by downloader name (curl / wget /
	// python3 / php / ruby on unix; powershell / pwsh on windows) and
	// renders the skip-TLS-verification flavour. Default for the
	// wizard's "Skip TLS verification" toggle when ON. The toggle is
	// ON by default because the install endpoint is most commonly
	// reached through a self-signed cert on first-boot deployments.
	InstallCommands map[string]string `json:"install_commands"`
	// InstallCommandsStrict is the same per-downloader map but
	// renders WITHOUT skip-cert flags — for prod deployments where
	// the install endpoint serves a system-trusted cert and the
	// admin wants a clean, MITM-resistant one-liner. The wizard
	// switches to this map when the operator turns off the "Skip
	// TLS verification" toggle.
	InstallCommandsStrict map[string]string `json:"install_commands_strict"`
	// BundleCommands and BundleCommandsStrict mirror install_commands
	// but emit the bundle shape: `platypus-agent "$(<fetch>)"` (unix)
	// or the PowerShell equivalent. Used when the operator picks the
	// "offline bundle" tab. Same single-use install token —
	// consuming the script form burns it just like consuming the
	// bundle form does.
	BundleCommands       map[string]string `json:"bundle_commands"`
	BundleCommandsStrict map[string]string `json:"bundle_commands_strict"`
	// BundleURL is the alternative single-string bootstrap form.
	// `curl -fsSL <bundle_url>` returns a `pinst_<base64>` token the
	// operator pastes straight into `platypus-agent`. Use when the
	// target machine can't pipe to a shell or when the operator
	// wants to inspect the bundle before running it. Same install
	// token underneath — choosing the script path or the bundle
	// path consumes it identically.
	BundleURL string `json:"bundle_url"`
}

// installListItem is the redacted view. It covers both unused + consumed
// tokens so the admin UI can show history.
type installListItem struct {
	DownloadID          string     `json:"download_id"`
	ProjectID           string     `json:"project_id"`
	IssuedByUser        string     `json:"issued_by_user"`
	IssuedAt            time.Time  `json:"issued_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	ServerEndpoint      string     `json:"server_endpoint"`
	TargetOS            string     `json:"target_os,omitempty"`
	TargetArch          string     `json:"target_arch,omitempty"`
	PATTTLSeconds       int        `json:"pat_ttl_seconds"`
	PATMaxUses          int        `json:"pat_max_uses"`
	PATBindingMachineID string     `json:"pat_binding_machine_id,omitempty"`
	PATDescription      string     `json:"pat_description,omitempty"`
	ConsumedAt          *time.Time `json:"consumed_at,omitempty"`
	ConsumedIP          string     `json:"consumed_ip,omitempty"`
	ConsumedPATID       string     `json:"consumed_pat_id,omitempty"`
	AutoApprove         bool       `json:"auto_approve"`
	Revoked             bool       `json:"revoked"`
	RevokedAt           *time.Time `json:"revoked_at,omitempty"`
	Status              string     `json:"status"`
}

func toInstallListItem(t *storage.InstallDownloadToken, now time.Time) installListItem {
	return installListItem{
		DownloadID:          t.DownloadID,
		ProjectID:           t.ProjectID,
		IssuedByUser:        t.IssuedByUser,
		IssuedAt:            t.IssuedAt,
		ExpiresAt:           t.ExpiresAt,
		ServerEndpoint:      t.ServerEndpoint,
		TargetOS:            t.TargetOS,
		TargetArch:          t.TargetArch,
		PATTTLSeconds:       t.PATTTLSeconds,
		PATMaxUses:          t.PATMaxUses,
		PATBindingMachineID: t.PATBindingMachineID,
		PATDescription:      t.PATDescription,
		ConsumedAt:          t.ConsumedAt,
		ConsumedIP:          t.ConsumedIP,
		ConsumedPATID:       t.ConsumedPATID,
		AutoApprove:         t.AutoApprove,
		Revoked:             t.Revoked,
		RevokedAt:           t.RevokedAt,
		Status:              string(t.Status(now)),
	}
}

// --- Handlers ------------------------------------------------------------

// Issue handles POST /projects/:pid/install-artifacts. Returns the
// plaintext download token + a pasteable curl command. Both exist only
// in this response; a follow-up GET returns the redacted list item.
func (h *InstallTokensHandler) Issue(c *gin.Context) {
	projectID := c.Param("pid")
	claims, _ := ClaimsFromContext(c)

	var req issueInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res, err := h.enroll.MintInstallArtifact(c.Request.Context(), enrollment.MintInstallArtifactInput{
		ProjectID:           projectID,
		IssuedByUser:        claims.UserID,
		ServerEndpoint:      req.ServerEndpoint,
		TargetOS:            req.TargetOS,
		TargetArch:          req.TargetArch,
		TTL:                 time.Duration(req.TTLSeconds) * time.Second,
		PATTTL:              time.Duration(req.PATTTLSeconds) * time.Second,
		PATMaxUses:          req.PATMaxUses,
		PATBindingMachineID: req.PATBindingMachineID,
		PATDescription:      req.PATDescription,
		AutoApprove:         req.AutoApprove,
	})
	if err != nil {
		h.audit(c, "install.issue", "install_download", "", projectID, req, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue install artifact"})
		return
	}
	h.audit(c, "install.issue", "install_download", res.DownloadID, projectID, req, "success", "")

	scriptInsecure, scriptStrict, scriptDefault, _ := h.renderInstallCommands(c.Request, res.PlaintextDownloadToken, res.TargetOS)
	bundleInsecure, bundleStrict, _, _ := h.renderBundleCommands(c.Request, res.PlaintextDownloadToken, res.TargetOS)
	c.JSON(http.StatusCreated, issueInstallResponse{
		DownloadID:            res.DownloadID,
		DownloadToken:         res.PlaintextDownloadToken,
		ExpiresAt:             res.ExpiresAt,
		ServerEndpoint:        res.ServerEndpoint,
		TargetOS:              res.TargetOS,
		TargetArch:            res.TargetArch,
		InstallCommand:        scriptDefault,
		InstallCommands:       scriptInsecure,
		InstallCommandsStrict: scriptStrict,
		BundleCommands:        bundleInsecure,
		BundleCommandsStrict:  bundleStrict,
		BundleURL:             h.distributorBase(c.Request) + "/api/v1/install/" + res.PlaintextDownloadToken + "?format=bundle",
	})
}

// distributorBase resolves the base URL the install endpoints sit
// under. Same precedence chain as renderInstallCommand uses; pulled
// out here so the bundle command renderer can share it.
func (h *InstallTokensHandler) distributorBase(req *http.Request) string {
	base := h.defaultDistributorBase
	if base != "" {
		return base
	}
	scheme := "http"
	if fwd := req.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	} else if req.TLS != nil {
		scheme = "https"
	}
	host := req.Host
	if fwd := req.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

// renderInstallCommands builds the family of bootstrap one-liners we
// hand back to the admin: one per supported downloader (curl, wget,
// python3, php, ruby on unix; powershell, pwsh on windows). The
// caller stores the whole map in install_commands so the FE picker
// can switch between them without re-issuing the install token, and
// also picks the family default for the legacy install_command
// field.
//
// Why a registry instead of OS-dispatch shell text inline: the
// macOS LibreSSL "unsupported algorithm" cascade taught us that one
// downloader is never enough — operators need a fallback when their
// system curl is broken against the server's TLS cert. Pre-rendering
// every variant lets the wizard offer the choice without an extra
// round-trip and keeps every clipboard-bound shape reviewable in one
// file (see internal/api/install_downloaders.go).
//
// The download token is base32-alphabet plus "." and "_" — safe to
// embed in a single-quoted shell string without escaping. The render
// helpers in the registry rely on that.
func (h *InstallTokensHandler) renderInstallCommands(
	req *http.Request, token, targetOS string,
) (insecure, strict map[string]string, insecureDefault, strictDefault string) {
	base := h.distributorBase(req)
	url := base + "/api/v1/install/" + token
	if downloaderOSFamily(targetOS) == osFamilyWindows {
		// Server-side os hint helps the distributor pick PS1 vs the
		// POSIX script when it serves the URL. Unix targets get the
		// default response, which is the auto-detecting POSIX script.
		url += "?os=windows"
	}
	insecureURL := url
	if strings.Contains(insecureURL, "?") {
		insecureURL += "&download_tls=insecure"
	} else {
		insecureURL += "?download_tls=insecure"
	}
	return renderCommandsForURLs(insecureURL, url, targetOS, false)
}

// renderBundleCommands is the bundle-shape sibling — same registry,
// same flavours, but each command runs `platypus-agent` on the
// fetched pinst_ token instead of piping it to a shell. The bundle
// URL uses `?format=bundle` so the distributor returns the bare
// pinst_ token instead of the install script. The os hint is
// orthogonal: bundle responses are OS-agnostic (the agent CLI parses
// the token).
func (h *InstallTokensHandler) renderBundleCommands(
	req *http.Request, token, targetOS string,
) (insecure, strict map[string]string, insecureDefault, strictDefault string) {
	bundleURL := h.distributorBase(req) + "/api/v1/install/" + token + "?format=bundle"
	return renderBundleCommandsFor(bundleURL, targetOS)
}

// List handles GET /projects/:pid/install-artifacts.
// ?include_inactive=true to also show revoked rows.
func (h *InstallTokensHandler) List(c *gin.Context) {
	projectID := c.Param("pid")
	includeInactive := c.Query("include_inactive") == "true"

	toks, err := h.db.InstallDownloadTokens().ListByProject(c.Request.Context(), projectID, includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list install tokens"})
		return
	}
	now := time.Now().UTC()
	out := make([]installListItem, 0, len(toks))
	for _, t := range toks {
		out = append(out, toInstallListItem(t, now))
	}
	c.JSON(http.StatusOK, gin.H{"install_artifacts": out})
}

// Get handles GET /projects/:pid/install-artifacts/:did. Includes the
// download-event history so admins can see exactly who curled it.
func (h *InstallTokensHandler) Get(c *gin.Context) {
	id := c.Param("did")
	ctx := c.Request.Context()
	tok, err := h.db.InstallDownloadTokens().Get(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "install artifact not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get install artifact"})
		return
	}
	events, err := h.db.InstallDownloadEvents().ListByDownload(ctx, id, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"install_artifact": toInstallListItem(tok, time.Now().UTC()),
		"events":           events,
	})
}

// Revoke handles DELETE /projects/:pid/install-artifacts/:did. Idempotent.
func (h *InstallTokensHandler) Revoke(c *gin.Context) {
	projectID := c.Param("pid")
	id := c.Param("did")
	claims, _ := ClaimsFromContext(c)
	reason := c.Query("reason")

	err := h.enroll.RevokeInstallDownload(c.Request.Context(), id, claims.UserID, reason)
	if errors.Is(err, storage.ErrNotFound) {
		h.audit(c, "install.revoke", "install_download", id, projectID, map[string]string{"reason": reason}, "denied", "not found")
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		h.audit(c, "install.revoke", "install_download", id, projectID, map[string]string{"reason": reason}, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke install artifact"})
		return
	}
	h.audit(c, "install.revoke", "install_download", id, projectID, map[string]string{"reason": reason}, "success", "")
	c.Status(http.StatusNoContent)
}

// audit funnels into the unified activities log. Swallows errors —
// losing an audit line is bad but not bad enough to fail the primary
// flow.
func (h *InstallTokensHandler) audit(c *gin.Context, action, targetType, targetID, projectID string, details interface{}, outcome, errText string) {
	pid := projectID
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryAdmin,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		TargetLabel: targetID,
		Outcome:     outcome,
		Error:       errText,
		Meta:        details,
	})
}

// RegisterV1InstallTokenRoutes mounts the admin surface. All routes are
// RequireProjectRole(admin) — handing out install links is privileged.
func RegisterV1InstallTokenRoutes(engine *gin.Engine, h *InstallTokensHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/install-artifacts")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		grp.POST("", h.Issue)
		grp.GET("", h.List)
		grp.GET("/:did", h.Get)
		grp.DELETE("/:did", h.Revoke)
	}

	// /api/v1/install/platforms drives the OS/arch picker on the issue
	// dialog. It's project-agnostic — the manifest is global server
	// state — so it sits outside the /projects/:pid group and only
	// requires authentication, not project admin.
	engine.GET("/api/v1/install/platforms",
		rbac.RequireAuth(),
		h.Platforms,
	)
}

// installPlatform is the slimmed-down (os, arch) shape the install
// dialog consumes — no SHA256, size, or storage key, since those are
// server-side concerns the UI doesn't need.
type installPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// installPlatformsResponse mirrors the Manifest envelope but exposes
// only what the dialog needs: the active channel, the version it pins,
// and the supported (os, arch) pairs. Empty Platforms is a valid
// response shape — it means no manifest has been published yet, and
// the UI renders a "publish first" hint.
type installPlatformsResponse struct {
	Channel   string            `json:"channel"`
	Version   string            `json:"version"`
	Platforms []installPlatform `json:"platforms"`
}

// Platforms handles GET /api/v1/install/platforms. Returns the (os, arch)
// pairs the active channel's manifest pins. When no distributor is
// configured (cfg.Distributor.Store.Endpoint blank) or no manifest has
// been published yet, returns 200 with an empty Platforms slice so the
// dialog can render a clear empty-state hint.
func (h *InstallTokensHandler) Platforms(c *gin.Context) {
	resp := installPlatformsResponse{Platforms: []installPlatform{}}
	d, ok := core.Ctx.Distributor.(*core.Distributor)
	if !ok || d == nil {
		c.JSON(http.StatusOK, resp)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	channel, version, artifacts := d.LivePlatforms(ctx)
	resp.Channel = channel
	resp.Version = version
	for _, a := range artifacts {
		resp.Platforms = append(resp.Platforms, installPlatform{OS: a.OS, Arch: a.Arch})
	}
	c.JSON(http.StatusOK, resp)
}
