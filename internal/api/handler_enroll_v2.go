package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// EnrollV2Handler implements POST /api/v1/agents/enroll, the
// protobuf-over-HTTPS endpoint that replaces the in-band Envelope
// AgentEnrollRequest. Agents PAT-authenticate here and receive a
// client certificate; the cert IS their long-term identity for all
// future connections (no session_token).
//
// On the happy path, exactly one leaf cert lands in the DB per call.
// The PAT is consumed atomically inside enrollment.RedeemEnrollmentToken;
// cert issuance happens afterwards and has its own ACID boundary —
// a cert issuance failure after PAT consumption is visible in the
// audit log but the PAT stays consumed. Re-try yields a new
// agent_id, which is what we want (stale aborted enrollments should
// not allow reuse of the originally-assigned id).
type EnrollV2Handler struct {
	enroll *enrollment.Service
	pki    *pki.Service
	// db is optional; when set, the handler upserts a hosts row on
	// every successful enrollment so the Web UI has something to
	// render even before the agent opens its first link. Keeping it
	// optional preserves the no-DB test paths in enroll_v2_test.go.
	db *storage.DB
	// approvalPolicy decides whether a freshly-enrolled host enrolls
	// straight to `approved` or has to wait for an admin click. Nil
	// means "always require approval unless the PAT carried
	// auto_approve" — this is the safer default for tests.
	approvalPolicy ApprovalPolicy
}

// ApprovalPolicy is the abstraction the v2 enroll handler consults to
// decide whether the global "require admin approval" toggle is on.
// Production wires this to settings.Registry.EnrollmentRequireApproval;
// tests can substitute a constant.
type ApprovalPolicy interface {
	EnrollmentRequireApproval() bool
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

// WithDB attaches a storage handle so the handler can upsert a hosts
// row on successful enrollment. Idempotent; passing nil disables the
// upsert (tests that don't care about hosts just skip calling it).
func (h *EnrollV2Handler) WithDB(db *storage.DB) *EnrollV2Handler {
	h.db = db
	return h
}

// WithApprovalPolicy attaches the live policy provider for the
// "require admin approval" toggle. Idempotent; nil restores the
// strict default (always require approval unless the PAT was
// auto-approved). Wired in main.go to settings.Registry.
func (h *EnrollV2Handler) WithApprovalPolicy(p ApprovalPolicy) *EnrollV2Handler {
	h.approvalPolicy = p
	return h
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
		// RedeemEnrollmentToken is skipped. We'll issue the cert explicitly from
		// the CSR below so the leaf carries the URI SAN bindings and
		// so the CSR signature is actually verified.
	}
	redeemed, err := h.enroll.RedeemEnrollmentToken(c.Request.Context(), req.Pat, rctx)
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

	// Persist a host row keyed on (project, agent_id) so the Web UI
	// immediately sees the fresh machine's hardware / OS details.
	// We swallow DB errors — a transient SQLite hiccup should not
	// break enrollment; the next agent reconnect will retry the
	// upsert via the link handler.
	//
	// Approval policy:
	//   - PAT.auto_approve=true → host lands in `approved` regardless of policy.
	//   - global require_approval=false → host lands in `approved`.
	//   - otherwise (the safe default) → host lands in `pending`,
	//     link handler will reject WS upgrades until an admin clicks
	//     Approve.
	initialApproval := storage.HostApprovalPending
	if redeemed.AutoApprove {
		initialApproval = storage.HostApprovalApproved
	} else if h.approvalPolicy != nil && !h.approvalPolicy.EnrollmentRequireApproval() {
		initialApproval = storage.HostApprovalApproved
	}
	if h.db != nil {
		if err := upsertHostFromEnroll(c.Request.Context(), h.db, redeemed, &req, initialApproval); err != nil {
			log.Warn("enroll: host upsert failed: agent=%s err=%v", redeemed.AgentID, err)
		}
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

// upsertHostFromEnroll turns the agent's EnrollRequest into a
// HostIdentity and pushes it through HostRepo.Upsert. The agent may
// report a platform machine_id (/etc/machine-id on Linux, IOPlatform
// UUID on macOS, MachineGuid on Windows) or a fallback "fp-…" hash;
// we distinguish by the "fp-" prefix so the UI can still show a
// "fingerprint fallback" badge on opaque hosts.
func upsertHostFromEnroll(ctx context.Context, db *storage.DB, redeemed *enrollment.RedeemResult, req *v2pb.EnrollRequest, initialApproval storage.HostApprovalStatus) error {
	host, err := upsertHostIdentity(ctx, db, redeemed, req, initialApproval)
	if err != nil {
		return err
	}
	// Stamp the operator's system-plugin allowlist onto the row so
	// the link-handler reconciler can diff against it on every
	// connect. Skip when the PAT was minted directly (no install
	// flow) — the host gets mandatory-core-only after reconcile.
	if len(redeemed.BaselinePluginIDs) > 0 {
		if err := db.Hosts().SetBaselinePluginIDs(ctx, host.ID, redeemed.BaselinePluginIDs); err != nil {
			return fmt.Errorf("set baseline plugin ids: %w", err)
		}
	}
	return nil
}

func upsertHostIdentity(ctx context.Context, db *storage.DB, redeemed *enrollment.RedeemResult, req *v2pb.EnrollRequest, initialApproval storage.HostApprovalStatus) (*storage.Host, error) {
	reported := req.GetMachineId()
	machineID := reported
	fingerprint := reported
	if strings.HasPrefix(reported, "fp-") {
		machineID = "" // fallback: no stable platform id
	}
	if fingerprint == "" {
		// Defend against an agent that sent neither — use the cert's
		// agent id so we still get a unique row per enrollment.
		fingerprint = "fp-agent-" + redeemed.AgentID
	}

	os := req.GetOs()
	if os == "" {
		// Older agents only send hostname; keep the column populated
		// with the platform family so the UI doesn't show "—".
		os = req.GetPlatform()
	}

	ident := &storage.HostIdentity{
		ProjectID:       redeemed.ProjectID,
		MachineID:       machineID,
		Fingerprint:     fingerprint,
		Hostname:        req.GetHostname(),
		OS:              os,
		SeenAt:          time.Now().UTC(),
		AgentID:         redeemed.AgentID,
		Arch:            req.GetArch(),
		Platform:        req.GetPlatform(),
		PlatformFamily:  req.GetPlatformFamily(),
		PlatformVersion: req.GetPlatformVersion(),
		KernelVersion:   req.GetKernelVersion(),
		CPUModel:        req.GetCpuModel(),
		NumCPU:          int(req.GetNumCpu()),
		MemTotalBytes:   int64(req.GetMemTotal()),
		CurrentUser:     req.GetCurrentUser(),
		Timezone:        req.GetTimezone(),
		PrimaryIP:       req.GetPrimaryIp(),
		PrimaryMAC:      req.GetPrimaryMac(),
		BootTimeUnix:    int64(req.GetBootTimeUnix()),
		BuildVersion:    req.GetBuildVersion(),
		BuildCommit:     req.GetBuildCommit(),
		BuildDate:       req.GetBuildDate(),
		ProtocolVersion: req.GetProtocolVersion(),
		MachineType:     req.GetMachineType(),
		ChassisType:     req.GetChassisType(),
		ProductVendor:   req.GetProductVendor(),
		ProductName:     req.GetProductName(),
		BIOSVendor:      req.GetBiosVendor(),
		BIOSVersion:     req.GetBiosVersion(),
		InitialApproval: initialApproval,
	}
	return db.Hosts().Upsert(ctx, ident)
}
