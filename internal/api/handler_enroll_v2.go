package api

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/pki"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// EnrollV2Handler implements POST /api/v1/agents/enroll, the
// protobuf-over-HTTPS endpoint that replaces the in-band Envelope
// AgentEnrollRequest. Agents PAT-authenticate here and receive a
// client certificate; the cert IS their long-term identity for all
// future connections (no session_token).
//
// On the happy path, exactly one leaf cert lands in the DB per call.
// The PAT is consumed atomically inside enrollment.RedeemPAT;
// cert issuance happens afterwards and has its own ACID boundary —
// a cert issuance failure after PAT consumption is visible in the
// audit log but the PAT stays consumed. Re-try yields a new
// agent_id, which is what we want (stale aborted enrollments should
// not allow reuse of the originally-assigned id).
type EnrollV2Handler struct {
	enroll *enrollment.Service
	pki    *pki.Service
}

// ContentTypeEnrollV2 is the MIME the endpoint accepts and
// returns. We keep v2 payloads on a distinct media type so any
// future HTTP caching / proxy layer treats them as opaque binary.
const ContentTypeEnrollV2 = "application/x-protobuf-platypus-v2"

// maxEnrollRequestBytes caps the request body. A CSR + identifying
// fields should be well under 8 KiB; anything larger is either a
// buggy client or a malicious one trying to DoS the parser.
const maxEnrollRequestBytes = 64 * 1024

func NewEnrollV2Handler(enroll *enrollment.Service, pkiSvc *pki.Service) *EnrollV2Handler {
	return &EnrollV2Handler{enroll: enroll, pki: pkiSvc}
}

// Enroll is the POST handler. Request body is a protobuf
// v2pb.EnrollRequest; response body is a protobuf v2pb.EnrollResponse.
// Errors short-circuit with the appropriate HTTP status:
//
//	400 malformed request or CSR
//	401 PAT rejected (malformed / revoked / exhausted / expired)
//	500 cert issuance failure (PAT already consumed; operator must mint a new one)
//	503 PKI not configured on this server
func (h *EnrollV2Handler) Enroll(c *gin.Context) {
	if h.pki == nil {
		c.String(http.StatusServiceUnavailable, "enroll: PKI not configured on this server")
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxEnrollRequestBytes+1))
	if err != nil {
		c.String(http.StatusBadRequest, "enroll: read body: %s", err)
		return
	}
	if len(body) > maxEnrollRequestBytes {
		c.String(http.StatusRequestEntityTooLarge, "enroll: request exceeds %d bytes", maxEnrollRequestBytes)
		return
	}

	var req v2pb.EnrollRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		c.String(http.StatusBadRequest, "enroll: parse request: %s", err)
		return
	}
	if req.Pat == "" {
		c.String(http.StatusUnauthorized, "enroll: PAT required (mTLS renewal not yet implemented)")
		return
	}
	if len(req.CsrPem) == 0 {
		c.String(http.StatusBadRequest, "enroll: csr_pem required")
		return
	}

	rctx := enrollment.RedeemContext{
		ClientIP:  c.ClientIP(),
		MachineID: req.MachineId,
		Hostname:  req.Hostname,
		// AgentPubKey deliberately empty: legacy cert-issuance via
		// RedeemPAT is skipped. We'll issue the cert explicitly from
		// the CSR below so the leaf carries the URI SAN bindings and
		// so the CSR signature is actually verified.
	}
	redeemed, err := h.enroll.RedeemPAT(c.Request.Context(), req.Pat, rctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "enroll: redeem PAT: %s", err)
		return
	}
	if redeemed.Outcome != "success" {
		// "malformed" / "unknown" / "revoked" / "exhausted" / "expired"
		// / "machine_mismatch". All of them map to 401.
		c.String(http.StatusUnauthorized, "enroll: PAT rejected: %s", redeemed.Outcome)
		return
	}

	issued, err := h.pki.IssueAgentLeafFromCSR(c.Request.Context(), pki.CSRInput{
		ProjectID: redeemed.ProjectID,
		AgentID:   redeemed.AgentID,
		CSRPEM:    req.CsrPem,
		Reason:    "enroll",
	})
	if err != nil {
		// PAT has been consumed at this point; the operator must
		// mint a new install token for this host to retry. We
		// surface the error detail so the agent's log can help
		// diagnose (e.g. wrong key algorithm, malformed CSR PEM).
		log.Error("enroll: IssueAgentLeafFromCSR: agent=%s project=%s err=%v",
			redeemed.AgentID, redeemed.ProjectID, err)
		c.String(http.StatusInternalServerError, "enroll: issue cert: %s", err)
		return
	}

	resp := &v2pb.EnrollResponse{
		CertPem:         []byte(issued.CertPEM),
		CaPem:           []byte(issued.CAPem),
		AgentId:         redeemed.AgentID,
		ProjectId:       redeemed.ProjectID,
		CertExpiresUnix: issued.NotAfter.Unix(),
	}
	out, err := proto.Marshal(resp)
	if err != nil {
		c.String(http.StatusInternalServerError, "enroll: marshal response: %s", err)
		return
	}
	c.Data(http.StatusOK, ContentTypeEnrollV2, out)
}

// RegisterV2AgentEnrollRoute wires the enroll endpoint. No auth
// middleware — the PAT inside the request body IS the credential.
// Once mTLS is mandatory in Phase II, we'll add a second entrypoint
// that accepts a renewal with client cert + fresh CSR.
func RegisterV2AgentEnrollRoute(engine *gin.Engine, h *EnrollV2Handler) {
	engine.POST("/api/v1/agents/enroll", h.Enroll)
}
