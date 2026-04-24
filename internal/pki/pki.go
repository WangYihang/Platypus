// Package pki holds the per-project certificate authority + cert
// issuance logic used by the enrollment flow. It is deliberately small:
// one self-signed Ed25519 root per project, short-lived leaf certs
// binding (agent_id, pubkey). No intermediate CAs, no ECDSA, no cross-
// signing. If the deployment ever needs those we add them here rather
// than scattering pem encoding across the codebase.
//
// The private key for each project CA is encrypted on disk using
// AES-256-GCM with a KEK supplied by the operator via PLATYPUS_CA_KEK
// (hex-encoded 32 bytes). Losing the DB alone can't forge certs;
// losing both KEK and DB can. Future work: wrap the CA key with HSM /
// cloud KMS so the KEK never sits on the server host.
package pki

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
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

	// KEKEnvVar is where the AES-256 key-encryption-key lives. Hex-
	// encoded (64 chars). Empty or missing → operator hasn't
	// configured CA; the service refuses to mint keys rather than
	// silently falling back to plaintext.
	KEKEnvVar = "PLATYPUS_CA_KEK"

	aesNonceLen = 12 // AES-GCM standard nonce length
	aesKeyLen   = 32 // AES-256
)

// ErrKEKMissing is returned when PLATYPUS_CA_KEK isn't set at the
// moment we need to seal / unseal a key. Handled as "CA not
// configured" at the admin layer.
var ErrKEKMissing = errors.New("pki: PLATYPUS_CA_KEK not set")

// ErrKEKMalformed is returned when the env var is set but isn't 32
// bytes of hex. Operators get a crisp error at startup rather than a
// mysterious 500 later.
var ErrKEKMalformed = errors.New("pki: PLATYPUS_CA_KEK must be 64 hex chars (32 bytes)")

// KEKPath, when non-empty, enables a dev-friendly fallback: if the
// PLATYPUS_CA_KEK env var is unset, readKEK reads the hex-encoded KEK
// from this file, and if the file is missing it generates a random
// KEK and writes it there (0600). The server main sets this to
// "<data-dir>/ca.kek" so `docker compose up` works with zero config.
// Tests and code paths that want the strict "env var required" old
// behavior leave it empty.
//
// Trade-off when the fallback is active: the KEK sits next to the
// SQLite file it's supposed to protect, so the CA private key is
// effectively plaintext to anyone who can read the data volume. For
// production, set PLATYPUS_CA_KEK explicitly (env takes precedence).
var KEKPath string

// autoKEKWarnOnce gates the one-shot WARN log emitted when readKEK
// auto-generates a KEK. Without it the warning would fire on every
// IssueAgentCert / RotateCA call.
var autoKEKWarnOnce sync.Once

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

// createCA generates a fresh Ed25519 keypair, self-signs a root cert,
// encrypts the private key under the KEK, and persists the row.
func (s *Service) createCA(ctx context.Context, projectID, createdBy string) (*storage.ProjectCA, error) {
	kek, err := readKEK()
	if err != nil {
		return nil, err
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("pki: generate CA key: %w", err)
	}
	certPEM, err := makeSelfSignedRoot(projectID, priv, pub)
	if err != nil {
		return nil, err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("pki: marshal CA privkey: %w", err)
	}
	nonce, ct, err := aesGCMSeal(kek, privDER)
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
	kek, err := readKEK()
	if err != nil {
		return nil, err
	}
	caPriv, err := unsealCAPriv(kek, ca.PrivKeyNonce, ca.PrivKeyCT)
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

// --- Low-level helpers ---------------------------------------------------

// readKEK resolves the key-encryption-key in priority order:
//  1. PLATYPUS_CA_KEK env var (the production path).
//  2. KEKPath file, if that package-level var is set (dev fallback).
//  3. Generate a random KEK and persist it at KEKPath, emitting a
//     one-shot WARN. Only reachable when KEKPath is set.
//
// If neither the env var nor KEKPath is set the function returns
// ErrKEKMissing, preserving the prior strict behavior for tests and
// any deployment that opts out of the file fallback.
func readKEK() ([]byte, error) {
	if raw := os.Getenv(KEKEnvVar); raw != "" {
		return decodeKEK(raw)
	}
	if KEKPath == "" {
		return nil, ErrKEKMissing
	}

	data, err := os.ReadFile(KEKPath)
	if err == nil {
		return decodeKEK(strings.TrimSpace(string(data)))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("pki: read kek file %q: %w", KEKPath, err)
	}

	kek := make([]byte, aesKeyLen)
	if _, err := rand.Read(kek); err != nil {
		return nil, fmt.Errorf("pki: generate kek: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(KEKPath), 0o700); err != nil {
		return nil, fmt.Errorf("pki: create kek dir: %w", err)
	}
	encoded := hex.EncodeToString(kek) + "\n"
	if err := os.WriteFile(KEKPath, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("pki: write kek file %q: %w", KEKPath, err)
	}
	autoKEKWarnOnce.Do(func() {
		log.L.Warn("auto_generated_ca_kek",
			"path", KEKPath,
			"hint", "set PLATYPUS_CA_KEK in production to keep the key outside the data volume",
		)
	})
	return kek, nil
}

// decodeKEK validates a hex-encoded KEK string and returns the raw
// 32-byte key, or ErrKEKMalformed on any decoding / length mismatch.
func decodeKEK(raw string) ([]byte, error) {
	kek, err := hex.DecodeString(raw)
	if err != nil || len(kek) != aesKeyLen {
		return nil, ErrKEKMalformed
	}
	return kek, nil
}

// aesGCMSeal encrypts plaintext under kek with a fresh nonce.
func aesGCMSeal(kek, plaintext []byte) (nonce, ciphertext []byte, err error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aesNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return nonce, ciphertext, nil
}

func aesGCMOpen(kek, nonce, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

func unsealCAPriv(kek, nonce, ct []byte) (ed25519.PrivateKey, error) {
	pkcs8, err := aesGCMOpen(kek, nonce, ct)
	if err != nil {
		return nil, fmt.Errorf("pki: unseal CA priv: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(pkcs8)
	if err != nil {
		return nil, fmt.Errorf("pki: parse CA priv: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("pki: CA key isn't Ed25519")
	}
	return priv, nil
}

// makeSelfSignedRoot produces a 10-year self-signed Ed25519 root with
// subject CN = "Platypus project <projectID>" and basicConstraints CA=true.
func makeSelfSignedRoot(projectID string, priv ed25519.PrivateKey, pub ed25519.PublicKey) (string, error) {
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return "", err
	}
	skid := sha256.Sum256(pub)
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

// signLeaf issues a leaf cert for an agent. The subject CN embeds the
// agent_id for easy identification in a packet capture / openssl view;
// the actual trust binding is the pubkey the cert commits to.
func signLeaf(caPriv ed25519.PrivateKey, caPEM, agentID string, pub ed25519.PublicKey, uris []*url.URL, serial int64, notBefore, notAfter time.Time) (string, error) {
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

// encodePublicKey renders an Ed25519 public key as a PKCS#8 PEM block.
// Used for the issued_certs.pubkey_pem column (admin UI diffs / export).
func encodePublicKey(pub ed25519.PublicKey) (string, error) {
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
	kek, err := readKEK()
	if err != nil {
		return nil, err
	}
	ca, err := s.db.ProjectCA().Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	caPriv, err := unsealCAPriv(kek, ca.PrivKeyNonce, ca.PrivKeyCT)
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
