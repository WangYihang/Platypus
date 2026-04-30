// Package mesh implements Platypus's self-organising agent overlay.
//
// Every node holds a project-CA-signed leaf cert; NodeID = the
// platypus://agent/<id> or platypus://server/<id> URI SAN. Mesh
// links ride mTLS-authenticated WebSocket upgrades on
// /api/v1/mesh/link served by the same http.Server that hosts the
// rest of the HTTPS surface. Project-CA membership is the admission
// gate, the URI SAN is the identity binding, and gossip + LSA
// distribution ride the WebSocket as length-prefixed protobuf
// MeshEnvelope frames.
package mesh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

const meshProtocolVersion = 1

// Identity bundles an Ed25519 keypair (used for LSA / gossip
// signatures) with the project-CA-signed leaf cert that anchors the
// NodeID.
type Identity struct {
	NodeID     string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
	CertPEM    []byte
}

// LoadIdentityFromCert constructs an Identity from a project-CA-signed
// agent leaf cert + its PKCS#8 private key.
func LoadIdentityFromCert(certPEM, keyPEM []byte) (*Identity, error) {
	cert, err := parseAgentLeafCert(certPEM)
	if err != nil {
		return nil, err
	}
	agentID, err := agentIDFromCert(cert)
	if err != nil {
		return nil, err
	}
	certPub, err := ed25519PublicKeyFromCert(cert)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("mesh: no PEM block in key material")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("mesh: parse PKCS#8 key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("mesh: private key is %T, want ed25519", parsed)
	}
	derivedPub, ok := priv.Public().(ed25519.PublicKey)
	if !ok || !bytes.Equal(derivedPub, certPub) {
		return nil, errors.New("mesh: private key does not match cert's public key")
	}
	return &Identity{
		NodeID:     agentID,
		PublicKey:  certPub,
		PrivateKey: priv,
		CertPEM:    append([]byte(nil), certPEM...),
	}, nil
}
