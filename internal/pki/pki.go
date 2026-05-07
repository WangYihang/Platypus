// Package pki holds the per-project certificate authority + cert
// issuance logic used by the enrollment flow. It is deliberately
// small: one self-signed ECDSA P-256 root per project, short-lived
// leaf certs binding (agent_id, pubkey). No intermediate CAs, no
// cross-signing.
//
// Why ECDSA P-256 (and not Ed25519, the previous choice): browsers,
// macOS LibreSSL curl, and Windows PowerShell 5.1's Schannel all
// fail to PARSE Ed25519-signed TLS cert chains — the cert signature
// algorithm is examined before any user-supplied
// ServerCertificateValidationCallback / -k callback runs, so the
// override can't bypass the failure. ECDSA P-256 is universally
// supported (CA/Browser Forum default, Let's Encrypt, every modern
// TLS stack since ~2010) and the leaf already uses it. Agent
// identities, mesh gossip keys, and the agent upgrade-manifest
// signing key all stay Ed25519 — none of those are TLS server cert
// signatures parsed by browsers / curl / PowerShell.
//
// The private key for each project CA is encrypted on disk using
// AES-256-GCM with a KEK supplied by the operator via PLATYPUS_CA_KEK
// (hex-encoded 32 bytes). Losing the DB alone can't forge certs;
// losing both KEK and DB can. Future work: wrap the CA key with HSM /
// cloud KMS so the KEK never sits on the server host.
package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"time"

	"github.com/WangYihang/Platypus/internal/cryptobox"
	"github.com/WangYihang/Platypus/internal/storage"
)

const (
	// CAValidity is how long a freshly minted root CA is trusted for.
	// Rotation is manual (admin replaces the row) — a 10y root is
	// long enough that the MVP doesn't need an automatic rotation job.
	CAValidity = 10 * 365 * 24 * time.Hour

	// DefaultAgentCertTTL is the leaf TTL for agent identity certs.
	// Matches DefaultSessionTTL in the enrollment package so one is
	// always valid when the other is — simplifies reasoning.
	DefaultAgentCertTTL = 30 * 24 * time.Hour

// KEKEnvVar / KEKPath are kept as alias spellings for back-compat
// with the pre-cryptobox call sites; the canonical names live in the
// cryptobox package. New code should reach for cryptobox.EnvVar /
// cryptobox.FilePath directly.
)

// KEKEnvVar is the env var name carrying the hex-encoded KEK. Alias
// for cryptobox.EnvVar; new callers should reach for cryptobox.
const KEKEnvVar = cryptobox.EnvVar

// ErrKEKMissing / ErrKEKMalformed are re-exports of the cryptobox
// errors so existing handler call sites keep matching the right
// sentinel without having to import a new package.
var (
	ErrKEKMissing   = cryptobox.ErrKEKMissing
	ErrKEKMalformed = cryptobox.ErrKEKMalformed
)

// Service wraps the storage + KEK plumbing. Stateless — safe to share
// across goroutines.
type Service struct {
	db *storage.DB
}

func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// IssueForAgent is the enrollment.PKIIssuer adapter. Errors here are
// swallowed at the enrollment layer (we'd rather issue a session
// without a cert than fail enrollment outright), but returning them
// makes the contract testable and gives future callers a lever.
func (s *Service) IssueForAgent(ctx context.Context, projectID, agentID string, pubkey []byte, reason string) (certPEM, caPEM string, err error) {
	if len(pubkey) == 0 {
		return "", "", nil
	}
	res, err := s.IssueAgentCert(ctx, IssueInput{
		ProjectID:   projectID,
		AgentID:     agentID,
		AgentPubKey: ed25519.PublicKey(pubkey),
		Reason:      reason,
	})
	if err != nil {
		return "", "", err
	}
	return res.CertPEM, res.CAPem, nil
}

// EnsureCA returns the project's CA, minting a fresh one on first
// access. The initial caller (`createdBy`) is recorded in project_ca
// for audit. Subsequent calls return the existing row.
func (s *Service) EnsureCA(ctx context.Context, projectID, createdBy string) (*storage.ProjectCA, error) {
	existing, err := s.db.ProjectCA().Get(ctx, projectID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	return s.createCA(ctx, projectID, createdBy)
}

// createCA generates a fresh ECDSA P-256 keypair, self-signs a root
// cert, encrypts the private key under the KEK, and persists the row.
func (s *Service) createCA(ctx context.Context, projectID, createdBy string) (*storage.ProjectCA, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("pki: generate CA key: %w", err)
	}
	certPEM, err := makeSelfSignedRoot(projectID, priv)
	if err != nil {
		return nil, err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("pki: marshal CA privkey: %w", err)
	}
	nonce, ct, err := cryptobox.Seal(privDER)
	if err != nil {
		return nil, err
	}

	ca := &storage.ProjectCA{
		ProjectID:     projectID,
		CertPEM:       certPEM,
		PrivKeyNonce:  nonce,
		PrivKeyCT:     ct,
		CreatedAt:     time.Now().UTC(),
		CreatedByUser: createdBy,
	}
	if err := s.db.ProjectCA().Create(ctx, ca); err != nil {
		// Two admins racing to mint a CA could both land here; the
		// PRIMARY KEY violation tells us the other one won, and we
		// simply return their row.
		if isUniqueViolation(err) {
			return s.db.ProjectCA().Get(ctx, projectID)
		}
		return nil, fmt.Errorf("pki: persist CA: %w", err)
	}
	return ca, nil
}

// IssueAgentCert signs a short-lived cert binding (agent_id, pubkey).
// Called during PAT redemption and session rotation; the cert PEM is
// embedded in AgentEnrollResponse so the agent persists it alongside
// its session token.
//
// The operation is transactional:
//
//  1. Allocate a fresh serial via ProjectCA.AllocateSerial (this is the
//     only row-level locking point; storage's SetMaxOpenConns(1)
//     serialises writers anyway, but BEGIN IMMEDIATE guarantees the
//     semantics explicitly).
//  2. Build and sign the leaf cert.
//  3. Insert the issued_certs row.
//
// If step 2 or 3 fails we roll back — no mismatched serial / cert
// leaks out.
type IssueInput struct {
	ProjectID    string
	AgentID      string
	AgentPubKey  ed25519.PublicKey
	TTL          time.Duration // default DefaultAgentCertTTL
	Reason       string        // "enroll" | "rotation" | "reissue" | "admin"
	IssuedByUser string        // "" for auto issuance during enrollment
}

type IssueResult struct {
	Serial    int64
	CertPEM   string
	CAPem     string
	NotBefore time.Time
	NotAfter  time.Time
}

// IssueAgentCert issues a leaf cert for the supplied agent pubkey. If
// the project's CA doesn't exist yet, it's minted first (with
// createdBy=IssuedByUser; blank when auto).
func (s *Service) IssueAgentCert(ctx context.Context, in IssueInput) (*IssueResult, error) {
	if len(in.AgentPubKey) != ed25519.PublicKeySize {
		return nil, errors.New("pki: agent pubkey wrong length")
	}
	if in.Reason == "" {
		in.Reason = "enroll"
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = DefaultAgentCertTTL
	}

	creator := in.IssuedByUser
	if creator == "" {
		creator = "system"
		// project_ca.created_by_user has FK to users(id). "system" isn't
		// a real user — we only need a valid user_id at auto-creation
		// time, and we don't have one here. Fall back to ensuring the
		// CA was pre-minted via admin flow; if not, return an error so
		// operators can see they haven't configured a project.
	}

	ca, err := s.db.ProjectCA().Get(ctx, in.ProjectID)
	if errors.Is(err, storage.ErrNotFound) {
		if in.IssuedByUser == "" {
			return nil, errors.New("pki: project CA not initialised; an admin must issue a cert manually or hit /api/v1/projects/:pid/ca to initialise")
		}
		if ca, err = s.createCA(ctx, in.ProjectID, creator); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if ca.RevokedAt != nil {
		return nil, errors.New("pki: project CA is revoked")
	}

	// Decrypt CA private key (kept in memory for the minimum necessary
	// span; Go's GC will zero it eventually, which is acceptable for
	// MVP — a forensic-grade solution would explicitly zero the slice).
	caPriv, err := unsealCAPriv(ca.PrivKeyNonce, ca.PrivKeyCT)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.ProjectCA().BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	serial, err := s.db.ProjectCA().AllocateSerial(ctx, tx, in.ProjectID)
	if err != nil {
		return nil, err
	}

	notBefore := time.Now().Add(-5 * time.Minute).UTC() // clock-skew grace
	notAfter := notBefore.Add(ttl + 5*time.Minute)
	uris, err := agentURISANs(in.AgentID, in.ProjectID)
	if err != nil {
		return nil, err
	}
	certPEM, err := signLeaf(caPriv, ca.CertPEM, in.AgentID, in.AgentPubKey, uris, serial, notBefore, notAfter)
	if err != nil {
		return nil, err
	}
	pubPEM, err := encodePublicKey(in.AgentPubKey)
	if err != nil {
		return nil, err
	}

	row := &storage.IssuedCert{
		Serial:       serial,
		ProjectID:    in.ProjectID,
		AgentID:      in.AgentID,
		CertPEM:      certPEM,
		PubKeyPEM:    pubPEM,
		IssuedAt:     time.Now().UTC(),
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		IssuedReason: in.Reason,
		IssuedByUser: in.IssuedByUser,
	}
	if err := s.db.IssuedCerts().InsertTx(ctx, tx, row); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	rollback = false
	return &IssueResult{
		Serial:    serial,
		CertPEM:   certPEM,
		CAPem:     ca.CertPEM,
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}, nil
}

// ServerCertResult bundles everything the ingress layer needs to
// stand up TLS: the server's leaf cert, its matching private key,
// and the project CA the chain anchors on.
type ServerCertResult struct {
	CertPEM string
	KeyPEM  string
	CAPem   string
}

// IssueServerCert mints a short-lived TLS server leaf signed by the
// project CA, with DNSNames / IPAddresses populated from `hosts` so
// the cert passes hostname verification. Returns the cert + its
// freshly-generated private key — agents pinning the same project CA
// will accept the cert without any further trust-store wiring.
//
// This is the counterpart to IssueAgentCert for the "who watches the
// watcher" problem: without it the ingress falls back to a stand-alone
// self-signed leaf which nothing in the agent trust graph chains to,
// and agents with PLATYPUS_PROJECT_CA set refuse the handshake.
func (s *Service) IssueServerCert(ctx context.Context, projectID string, hosts []string, issuedByUser string) (*ServerCertResult, error) {
	if len(hosts) == 0 {
		return nil, errors.New("pki: IssueServerCert: hosts must be non-empty")
	}
	ca, err := s.db.ProjectCA().Get(ctx, projectID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, errors.New("pki: IssueServerCert: project CA not initialised; call EnsureCA first")
	}
	if err != nil {
		return nil, err
	}
	if ca.RevokedAt != nil {
		return nil, errors.New("pki: IssueServerCert: project CA is revoked")
	}

	caPriv, err := unsealCAPriv(ca.PrivKeyNonce, ca.PrivKeyCT)
	if err != nil {
		return nil, err
	}

	// ECDSA P-256 across the board (CA + leaf) — universally
	// supported by browsers / curl / PowerShell. See the package doc
	// for why Ed25519 was abandoned for the CA.
	leafPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("pki: IssueServerCert keygen: %w", err)
	}
	leafPub := leafPriv.Public().(*ecdsa.PublicKey)

	tx, err := s.db.ProjectCA().BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	serial, err := s.db.ProjectCA().AllocateSerial(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}

	notBefore := time.Now().Add(-5 * time.Minute).UTC()
	notAfter := notBefore.Add(DefaultAgentCertTTL + 5*time.Minute)
	// Mesh peer identity rides in the same cert via a URI SAN
	// (platypus://server/<projectID>). Mesh peers extract NodeID
	// from this after mTLS.
	uris, err := serverURISANs(projectID)
	if err != nil {
		return nil, err
	}
	certPEM, err := signServerLeaf(caPriv, ca.CertPEM, hosts, uris, leafPub, serial, notBefore, notAfter)
	if err != nil {
		return nil, err
	}
	pubPEM, err := encodePublicKey(leafPub)
	if err != nil {
		return nil, err
	}

	row := &storage.IssuedCert{
		Serial:       serial,
		ProjectID:    projectID,
		AgentID:      "server",
		CertPEM:      certPEM,
		PubKeyPEM:    pubPEM,
		IssuedAt:     time.Now().UTC(),
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		IssuedReason: "admin",
		IssuedByUser: issuedByUser,
	}
	if err := s.db.IssuedCerts().InsertTx(ctx, tx, row); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	rollback = false

	keyDER, err := x509.MarshalPKCS8PrivateKey(leafPriv)
	if err != nil {
		return nil, fmt.Errorf("pki: IssueServerCert marshal key: %w", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))

	return &ServerCertResult{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		CAPem:   ca.CertPEM,
	}, nil
}

// signServerLeaf stamps DNS / IP SANs for hostname verification plus
// URI SANs that double as the server's mesh-peer identity. hosts can
// be a mix of hostnames and numeric IPs; net.ParseIP decides the
// bucket. Both leaf and CA are ECDSA P-256; the resulting leaf's
// SignatureAlgorithm is ECDSAWithSHA256, which every Schannel /
// Secure Transport / OpenSSL build parses fine.
//
// The cert advertises BOTH serverAuth and clientAuth EKUs. serverAuth
// is for the obvious ingress role (browsers, agents dialing in);
// clientAuth lets the server act as an mTLS client when it dials an
// agent's mesh peer listener (the NAT-traversal case where an
// isolated agent reaches the server through a peer relay — see the
// /api/v1/mesh/link handler).
func signServerLeaf(caPriv *ecdsa.PrivateKey, caPEM string, hosts []string, uris []*url.URL, pub *ecdsa.PublicKey, serial int64, notBefore, notAfter time.Time) (string, error) {
	caCert, err := parseCAFromPEM(caPEM)
	if err != nil {
		return "", err
	}

	var dns []string
	var ips []net.IP
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
		} else {
			dns = append(dns, h)
		}
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:   "platypus-ingress",
			Organization: []string{"Platypus"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		DNSNames:              dns,
		IPAddresses:           ips,
		URIs:                  uris,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, pub, caPriv)
	if err != nil {
		return "", fmt.Errorf("pki: sign server leaf: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

// serverURISANs builds the URI SAN list for the server's
// ingress+mesh leaf. The single SAN platypus://server/<projectID>
// gives mesh peers a stable NodeID to bind to: when an agent's mesh
// dialer completes mTLS against the server, it extracts this URI
// from the verified peer cert and uses "server-<projectID>" as the
// peer's mesh identity.
func serverURISANs(projectID string) ([]*url.URL, error) {
	if projectID == "" {
		return nil, errors.New("pki: serverURISANs: empty projectID")
	}
	u, err := url.Parse("platypus://server/" + projectID)
	if err != nil {
		return nil, fmt.Errorf("pki: serverURISANs: parse: %w", err)
	}
	return []*url.URL{u}, nil
}

// --- Low-level helpers ---------------------------------------------------

// unsealCAPriv decrypts the stored CA private key blob using the
// process-wide KEK and parses it back into an *ecdsa.PrivateKey.
// KEK loading lives in cryptobox; this wrapper preserves the strict
// type checks (P-256 only) that callers depend on.
func unsealCAPriv(nonce, ct []byte) (*ecdsa.PrivateKey, error) {
	pkcs8, err := cryptobox.Open(nonce, ct)
	if err != nil {
		return nil, fmt.Errorf("pki: unseal CA priv: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(pkcs8)
	if err != nil {
		return nil, fmt.Errorf("pki: parse CA priv: %w", err)
	}
	priv, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("pki: project CA is not ECDSA P-256 (got %T; legacy key types are no longer supported, delete the project_ca row to mint a fresh CA)", parsed)
	}
	if priv.Curve != elliptic.P256() {
		return nil, fmt.Errorf("pki: project CA ECDSA curve must be P-256 (got %s)", priv.Curve.Params().Name)
	}
	return priv, nil
}

// makeSelfSignedRoot produces a 10-year self-signed ECDSA P-256 root
// with subject CN = "Platypus project <projectID>" and
// basicConstraints CA=true. SKID is the SHA-256 hash of the
// SubjectPublicKeyInfo's BIT STRING, truncated to 20 bytes (the
// standard form for non-RSA keys).
func makeSelfSignedRoot(projectID string, priv *ecdsa.PrivateKey) (string, error) {
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return "", err
	}
	pub := &priv.PublicKey
	spki, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("pki: marshal CA pubkey: %w", err)
	}
	skid := sha256.Sum256(spki)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Platypus project " + projectID,
			Organization: []string{"Platypus"},
		},
		NotBefore:             time.Now().Add(-5 * time.Minute).UTC(),
		NotAfter:              time.Now().Add(CAValidity).UTC(),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		SubjectKeyId:          skid[:20],
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		return "", fmt.Errorf("pki: create root: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

// signLeaf issues a leaf cert for an agent. The agent's pubkey stays
// Ed25519; the resulting leaf is Ed25519-public-key + ECDSAWithSHA256
// signed-by-CA, which Go x509 stitches together fine. The subject CN
// embeds the agent_id for easy identification in a packet capture /
// openssl view; the actual trust binding is the pubkey the cert
// commits to.
func signLeaf(caPriv *ecdsa.PrivateKey, caPEM, agentID string, pub ed25519.PublicKey, uris []*url.URL, serial int64, notBefore, notAfter time.Time) (string, error) {
	caCert, err := parseCAFromPEM(caPEM)
	if err != nil {
		return "", err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:   agentID,
			Organization: []string{"Platypus"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		URIs:                  uris,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, pub, caPriv)
	if err != nil {
		return "", fmt.Errorf("pki: sign leaf: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

func parseCAFromPEM(caPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(caPEM))
	if block == nil {
		return nil, errors.New("pki: CA PEM decode failed")
	}
	return x509.ParseCertificate(block.Bytes)
}

// encodePublicKey renders any supported public key (Ed25519 for
// agent identities, ECDSA for the ingress server leaf) as a PKCS#8
// PEM block. Used for the issued_certs.pubkey_pem column.
func encodePublicKey(pub any) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

// BuildCRL generates a DER-encoded x509 CRL listing revoked-but-not-
// yet-expired certs. MVP uses the v1 CRL format (IsLegacyCRL through
// CreateRevocationList) — consumers that only speak RFC5280 v2 CRLs
// will upgrade when we add the extension.
func (s *Service) BuildCRL(ctx context.Context, projectID string) ([]byte, error) {
	ca, err := s.db.ProjectCA().Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	caPriv, err := unsealCAPriv(ca.PrivKeyNonce, ca.PrivKeyCT)
	if err != nil {
		return nil, err
	}
	caCert, err := parseCAFromPEM(ca.CertPEM)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	revoked, err := s.db.IssuedCerts().ListRevokedLive(ctx, projectID, now)
	if err != nil {
		return nil, err
	}
	var entries []x509.RevocationListEntry
	for _, r := range revoked {
		revokedAt := now
		if r.RevokedAt != nil {
			revokedAt = *r.RevokedAt
		}
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   big.NewInt(r.Serial),
			RevocationTime: revokedAt,
		})
	}
	tmpl := &x509.RevocationList{
		RevokedCertificateEntries: entries,
		Number:                    big.NewInt(now.Unix()),
		ThisUpdate:                now,
		NextUpdate:                now.Add(24 * time.Hour),
	}
	return x509.CreateRevocationList(rand.Reader, tmpl, caCert, caPriv)
}

// isUniqueViolation mirrors the helper in internal/storage.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return containsUnique(err.Error())
}

func containsUnique(s string) bool {
	return len(s) > 0 && (s == "UNIQUE constraint failed" ||
		(len(s) >= 24 && s[:24] == "UNIQUE constraint failed"))
}
