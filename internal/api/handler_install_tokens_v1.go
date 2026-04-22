package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

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

	// defaultDistributorBase is what we prepend to "/install/<id>" when
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
	PATBindingMachineID string `json:"pat_binding_machine_id"`
	PATDescription      string `json:"pat_description"`
}

// issueInstallResponse is the only place the plaintext download token
// appears in the API. The `install_command` field is a convenience:
// a ready-to-paste curl that admins can drop into chat / terminal.
type issueInstallResponse struct {
	DownloadID     string    `json:"download_id"`
	DownloadToken  string    `json:"download_token"` // dl_<id>.<secret>
	ExpiresAt      time.Time `json:"expires_at"`
	ServerEndpoint string    `json:"server_endpoint"`
	TargetOS       string    `json:"target_os,omitempty"`
	TargetArch     string    `json:"target_arch,omitempty"`
	InstallCommand string    `json:"install_command"` // "curl -fsSL ... | sh"
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
	PATBindingMachineID string     `json:"pat_binding_machine_id,omitempty"`
	PATDescription      string     `json:"pat_description,omitempty"`
	ConsumedAt          *time.Time `json:"consumed_at,omitempty"`
	ConsumedIP          string     `json:"consumed_ip,omitempty"`
	ConsumedPATID       string     `json:"consumed_pat_id,omitempty"`
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
		PATBindingMachineID: t.PATBindingMachineID,
		PATDescription:      t.PATDescription,
		ConsumedAt:          t.ConsumedAt,
		ConsumedIP:          t.ConsumedIP,
		ConsumedPATID:       t.ConsumedPATID,
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
		PATBindingMachineID: req.PATBindingMachineID,
		PATDescription:      req.PATDescription,
	})
	if err != nil {
		h.audit(c, "install.issue", "install_download", "", projectID, req, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue install artifact"})
		return
	}
	h.audit(c, "install.issue", "install_download", res.DownloadID, projectID, req, "success", "")

	cmd := h.renderInstallCommand(c.Request, res.PlaintextDownloadToken)
	c.JSON(http.StatusCreated, issueInstallResponse{
		DownloadID:     res.DownloadID,
		DownloadToken:  res.PlaintextDownloadToken,
		ExpiresAt:      res.ExpiresAt,
		ServerEndpoint: res.ServerEndpoint,
		TargetOS:       res.TargetOS,
		TargetArch:     res.TargetArch,
		InstallCommand: cmd,
	})
}

// renderInstallCommand builds the curl command we return to the admin.
// Preference order for the distributor base URL:
//
//  1. Handler's configured defaultDistributorBase (from server.yml).
//  2. X-Forwarded-Proto + Host headers (handy for reverse proxies).
//  3. Plain http://Host (falls through to the incoming request's view).
//
// The resulting command is safe to copy into a terminal — no shell
// escaping is required because the download token only contains
// base32-alphabet characters plus "." and "_".
func (h *InstallTokensHandler) renderInstallCommand(req *http.Request, token string) string {
	base := h.defaultDistributorBase
	if base == "" {
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
		base = scheme + "://" + host
	}
	return "curl -fsSL " + base + "/install/" + token + " | sh"
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
}
