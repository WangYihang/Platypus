package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// EnrollOptions carries the inputs needed for a single POST to
// /api/v1/agents/enroll. ProjectCA / InsecureSkipVerify control how
// the HTTPS chain to the server is validated: on a first-run agent
// that has the install-script-provided CA in PLATYPUS_PROJECT_CA,
// ProjectCA is populated and InsecureSkipVerify stays false; when
// the operator runs the agent with no CA material (local dev) they
// can opt into InsecureSkipVerify.
//
// HTTPClient is an optional override so tests can inject a stub
// server's client — production callers leave it nil and get a
// freshly-built client with a 30s timeout.
type EnrollOptions struct {
	ServerURL          string
	PAT                string
	Hostname           string
	MachineID          string
	ProjectCA          *x509.CertPool
	InsecureSkipVerify bool
	HTTPClient         *http.Client

	// Build identity, sourced from pkg/version. All optional; the
	// server treats them as advisory display fields.
	BuildVersion string // semver
	Commit       string // short git SHA
	BuildDate    string // RFC3339

	// Wire-protocol version this binary speaks. Sourced from
	// internal/link.ProtocolVersion. Zero means the agent didn't
	// advertise a version (pre-versioning binary); the server may
	// treat that as v1 for compatibility decisions.
	ProtocolVersion uint32

	// SysInfo is an optional agent-collected system snapshot the
	// server persists on the hosts row so the Web UI has something
	// to show even when the agent is offline. Nil → server stores
	// only the legacy hostname / machine_id fields.
	SysInfo *v2pb.SysInfoResponse
}

// EnrollResult is what Enroll returns on success: the freshly-
// minted identity material ready to hand to SaveIdentity, plus the
// server-reported agent/project identifiers and cert expiry.
// Distinct from the v1 in-band EnrollmentResult in enrollment.go.
type EnrollResult struct {
	Identity  Identity
	AgentID   string
	ProjectID string
	ExpiresAt time.Time
	// PrivateKey is also stored inside Identity.PrivateKey; exposed
	// here as a convenience so callers that only want the key don't
	// have to reach through the struct.
	PrivateKey ed25519.PrivateKey
}

// ErrEnrollBadResponse is returned when the server replies with an
// HTTP status that isn't a well-defined enroll outcome. Included in
// the error chain so callers can distinguish "network / server
// problem" from "PAT is invalid".
var ErrEnrollBadResponse = errors.New("agent: enroll: unexpected server response")

// Enroll performs one PAT-authenticated enrollment. It builds a
// fresh Ed25519 keypair + CSR, sends the protobuf request, parses
// the response, and returns everything SaveIdentity needs.
//
// On any non-2xx HTTP status we surface the status code and body so
// operators can see exactly what the server said (PAT rejected,
// PKI misconfigured, etc.) without having to turn on server-side
// trace logging.
func Enroll(ctx context.Context, opts EnrollOptions) (*EnrollResult, error) {
	if opts.ServerURL == "" {
		return nil, errors.New("agent: Enroll: ServerURL required")
	}
	if opts.PAT == "" {
		return nil, errors.New("agent: Enroll: PAT required")
	}

	csrPEM, priv, err := GenerateCSR()
	if err != nil {
		return nil, fmt.Errorf("agent: Enroll generate CSR: %w", err)
	}

	enrollReq := &v2pb.EnrollRequest{
		Pat:             opts.PAT,
		CsrPem:          csrPEM,
		Hostname:        opts.Hostname,
		MachineId:       opts.MachineID,
		BuildVersion:    opts.BuildVersion,
		Commit:          opts.Commit,
		BuildDate:       opts.BuildDate,
		ProtocolVersion: opts.ProtocolVersion,
	}
	if s := opts.SysInfo; s != nil {
		// Server persists these as advisory fields on the hosts row.
		// Match only the stable subset so the server doesn't have to
		// validate dynamic metrics it won't use.
		enrollReq.Os = s.Os
		enrollReq.Arch = s.Arch
		enrollReq.Platform = s.Platform
		enrollReq.PlatformFamily = s.PlatformFamily
		enrollReq.PlatformVersion = s.PlatformVersion
		enrollReq.KernelVersion = s.KernelVersion
		enrollReq.NumCpu = s.NumCpu
		enrollReq.MemTotal = s.MemTotal
		enrollReq.CpuModel = s.CpuModel
		enrollReq.CurrentUser = s.CurrentUser
		enrollReq.Timezone = s.Timezone
		enrollReq.PrimaryIp = s.PrimaryIp
		enrollReq.PrimaryMac = s.PrimaryMac
		enrollReq.BootTimeUnix = s.BootTimeUnix
		enrollReq.MachineType = s.MachineType
		enrollReq.ChassisType = s.ChassisType
		enrollReq.ProductVendor = s.ProductVendor
		enrollReq.ProductName = s.ProductName
		enrollReq.BiosVendor = s.BiosVendor
		enrollReq.BiosVersion = s.BiosVersion
	}
	reqBody, err := proto.Marshal(enrollReq)
	if err != nil {
		return nil, fmt.Errorf("agent: Enroll marshal request: %w", err)
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion:         tls.VersionTLS12,
					RootCAs:            opts.ProjectCA,
					InsecureSkipVerify: opts.InsecureSkipVerify, //nolint:gosec // opt-in via opts
					// Server's unified ingress dispatches by ALPN
					// (internal/ingress/dispatcher.go). Without an
					// explicit NextProtos the server's default-case
					// closes the connection silently — a quiet EOF
					// that took several debugging sessions to track
					// down. Pin http/1.1 because http.Transport here
					// isn't wrapped with the http2 configurator;
					// advertising "h2" would make the server speak
					// h2 but the client would try to parse h1,
					// producing a malformed-response error.
					NextProtos: []string{"http/1.1"},
				},
			},
		}
	}

	url := opts.ServerURL + "/api/v1/agents/enroll"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("agent: Enroll build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf-platypus-v2")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent: Enroll POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("agent: Enroll read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// The server's text bodies are short, human-readable messages
		// (e.g. "enroll: PAT rejected: expired"). Including them verbatim
		// makes agent logs actionable without leaking anything the
		// operator didn't already have.
		return nil, fmt.Errorf("%w: status %d: %s",
			ErrEnrollBadResponse, resp.StatusCode, bytes.TrimSpace(respBody))
	}

	var parsed v2pb.EnrollResponse
	if err := proto.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("agent: Enroll parse response: %w", err)
	}
	if parsed.AgentId == "" || len(parsed.CertPem) == 0 || len(parsed.CaPem) == 0 {
		return nil, fmt.Errorf("%w: response missing agent_id / cert_pem / ca_pem",
			ErrEnrollBadResponse)
	}

	return &EnrollResult{
		Identity: Identity{
			PrivateKey: priv,
			CertPEM:    parsed.CertPem,
			CAPEM:      parsed.CaPem,
		},
		PrivateKey: priv,
		AgentID:    parsed.AgentId,
		ProjectID:  parsed.ProjectId,
		ExpiresAt:  time.Unix(parsed.CertExpiresUnix, 0),
	}, nil
}
