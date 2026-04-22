package api

import (
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// CAHandler owns the Phase 4 PKI admin surface: initialise / fetch the
// project CA, list and revoke issued certs, and publish a CRL that
// mesh / mTLS consumers will read once Phase 5 wires them up. PKI is
// an optional subsystem; when `PLATYPUS_CA_KEK` isn't configured the
// endpoints respond with 503 rather than misleading 500s.
type CAHandler struct {
	db  *storage.DB
	svc *pki.Service
}

func NewCAHandler(db *storage.DB, svc *pki.Service) *CAHandler {
	return &CAHandler{db: db, svc: svc}
}

// --- Response shapes ------------------------------------------------

type caResponse struct {
	ProjectID     string    `json:"project_id"`
	CertPEM       string    `json:"cert_pem"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedByUser string    `json:"created_by_user"`
	SerialCounter int64     `json:"serial_counter"`
}

type issuedCertItem struct {
	Serial        int64      `json:"serial"`
	ProjectID     string     `json:"project_id"`
	AgentID       string     `json:"agent_id,omitempty"`
	CertPEM       string     `json:"cert_pem"`
	PubKeyPEM     string     `json:"pubkey_pem"`
	IssuedAt      time.Time  `json:"issued_at"`
	NotBefore     time.Time  `json:"not_before"`
	NotAfter      time.Time  `json:"not_after"`
	IssuedReason  string     `json:"issued_reason"`
	IssuedByUser  string     `json:"issued_by_user,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	RevokedReason string     `json:"revoked_reason,omitempty"`
	Active        bool       `json:"active"`
}

func toCertItem(c *storage.IssuedCert, now time.Time) issuedCertItem {
	return issuedCertItem{
		Serial:        c.Serial,
		ProjectID:     c.ProjectID,
		AgentID:       c.AgentID,
		CertPEM:       c.CertPEM,
		PubKeyPEM:     c.PubKeyPEM,
		IssuedAt:      c.IssuedAt,
		NotBefore:     c.NotBefore,
		NotAfter:      c.NotAfter,
		IssuedReason:  c.IssuedReason,
		IssuedByUser:  c.IssuedByUser,
		RevokedAt:     c.RevokedAt,
		RevokedReason: c.RevokedReason,
		Active:        c.IsActive(now),
	}
}

// --- Handlers -------------------------------------------------------

// GetCA handles GET /api/v1/projects/:pid/ca. Returns 404 if no CA
// exists yet; unlike the agent-facing issuance flow we don't auto-mint
// here — admins explicitly initialise via POST.
func (h *CAHandler) GetCA(c *gin.Context) {
	projectID := c.Param("pid")
	ca, err := h.db.ProjectCA().Get(c.Request.Context(), projectID)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "project CA not initialised"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get ca"})
		return
	}
	c.JSON(http.StatusOK, caResponse{
		ProjectID:     ca.ProjectID,
		CertPEM:       ca.CertPEM,
		CreatedAt:     ca.CreatedAt,
		CreatedByUser: ca.CreatedByUser,
		SerialCounter: ca.SerialCounter,
	})
}

// InitCA handles POST /api/v1/projects/:pid/ca. Idempotent — calling
// it again returns the existing CA rather than a 409. This matches
// the EnsureCA semantics and lets operators re-fetch the CertPEM if
// they lost the earlier response.
func (h *CAHandler) InitCA(c *gin.Context) {
	projectID := c.Param("pid")
	claims, _ := ClaimsFromContext(c)

	ca, err := h.svc.EnsureCA(c.Request.Context(), projectID, claims.UserID)
	if err != nil {
		if errors.Is(err, pki.ErrKEKMissing) || errors.Is(err, pki.ErrKEKMalformed) {
			h.audit(c, "ca.init", "project_ca", projectID, projectID, nil, "error", err.Error())
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PKI not configured: " + err.Error()})
			return
		}
		h.audit(c, "ca.init", "project_ca", projectID, projectID, nil, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "init ca"})
		return
	}
	h.audit(c, "ca.init", "project_ca", projectID, projectID, nil, "success", "")
	c.JSON(http.StatusOK, caResponse{
		ProjectID:     ca.ProjectID,
		CertPEM:       ca.CertPEM,
		CreatedAt:     ca.CreatedAt,
		CreatedByUser: ca.CreatedByUser,
		SerialCounter: ca.SerialCounter,
	})
}

// ListCerts handles GET /api/v1/projects/:pid/certs.
// ?active=true (default) hides revoked and expired rows.
func (h *CAHandler) ListCerts(c *gin.Context) {
	projectID := c.Param("pid")
	activeOnly := c.DefaultQuery("active", "true") == "true"

	now := time.Now().UTC()
	certs, err := h.db.IssuedCerts().ListByProject(c.Request.Context(), projectID, activeOnly, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list certs"})
		return
	}
	out := make([]issuedCertItem, 0, len(certs))
	for _, cc := range certs {
		out = append(out, toCertItem(cc, now))
	}
	c.JSON(http.StatusOK, gin.H{"certs": out})
}

// RevokeCert handles DELETE /api/v1/projects/:pid/certs/:serial.
// Idempotent; revoking already-revoked returns 204.
func (h *CAHandler) RevokeCert(c *gin.Context) {
	projectID := c.Param("pid")
	serialStr := c.Param("serial")
	serial, err := strconv.ParseInt(serialStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid serial"})
		return
	}
	claims, _ := ClaimsFromContext(c)
	reason := c.Query("reason")

	err = h.db.IssuedCerts().Revoke(c.Request.Context(), projectID, serial, claims.UserID, reason, time.Now().UTC())
	target := projectID + "/" + serialStr
	if errors.Is(err, storage.ErrNotFound) {
		h.audit(c, "cert.revoke", "issued_cert", target, projectID, map[string]string{"reason": reason}, "denied", "not found")
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		h.audit(c, "cert.revoke", "issued_cert", target, projectID, map[string]string{"reason": reason}, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke cert"})
		return
	}
	h.audit(c, "cert.revoke", "issued_cert", target, projectID, map[string]string{"reason": reason}, "success", "")
	c.Status(http.StatusNoContent)
}

// CRL handles GET /api/v1/projects/:pid/crl. Returns a DER-encoded
// RFC5280 CRL signed by the project CA. Format is negotiated via
// `?format=pem|der`; default DER matches what most TLS libraries want.
func (h *CAHandler) CRL(c *gin.Context) {
	projectID := c.Param("pid")
	ctx := c.Request.Context()

	derCRL, err := h.svc.BuildCRL(ctx, projectID)
	if err != nil {
		if errors.Is(err, pki.ErrKEKMissing) || errors.Is(err, pki.ErrKEKMalformed) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PKI not configured: " + err.Error()})
			return
		}
		if errors.Is(err, storage.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project CA not initialised"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "build crl"})
		return
	}

	if c.Query("format") == "pem" {
		c.Header("Content-Type", "application/x-pem-file")
		_ = pem.Encode(c.Writer, &pem.Block{Type: "X509 CRL", Bytes: derCRL})
		return
	}
	c.Header("Content-Type", "application/pkix-crl")
	_, _ = c.Writer.Write(derCRL)
}

// audit mirrors the helper used by PAT / install handlers.
func (h *CAHandler) audit(c *gin.Context, action, targetType, targetID, projectID string, details interface{}, outcome, errText string) {
	claims, _ := ClaimsFromContext(c)
	blob, _ := json.Marshal(details)
	_ = h.db.AdminAuditLog().Record(c.Request.Context(), &storage.AdminAuditEvent{
		At:         time.Now().UTC(),
		ActorUser:  claims.UserID,
		ActorIP:    c.ClientIP(),
		ActorUA:    c.Request.UserAgent(),
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		ProjectID:  projectID,
		Details:    string(blob),
		Outcome:    outcome,
		Error:      errText,
	})
}

// RegisterV1CARoutes wires the CA surface. Admin-only.
func RegisterV1CARoutes(engine *gin.Engine, h *CAHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		grp.GET("/ca", h.GetCA)
		grp.POST("/ca", h.InitCA)
		grp.GET("/certs", h.ListCerts)
		grp.DELETE("/certs/:serial", h.RevokeCert)
		grp.GET("/crl", h.CRL)
	}
}
