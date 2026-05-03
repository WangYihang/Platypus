package plugin

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jedisct1/go-minisign"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if pk.SignatureAlgorithm != [2]byte{'E', 'd'} {
		t.Errorf("pk algo = %v", pk.SignatureAlgorithm)
	}
	if pk.KeyId != sk.KeyID {
		t.Errorf("keyId mismatch between sk and pk")
	}

	data := []byte("the quick brown fox jumps over the lazy dog")
	sig, err := Sign(sk, data, "test")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := VerifyWasm(pk, data, sig); err != nil {
		t.Errorf("verify roundtrip: %v", err)
	}
}

func TestSign_TamperedDataRejected(t *testing.T) {
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	data := []byte("hello")
	sig, err := Sign(sk, data, "")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tampered := []byte("hellp")
	if err := VerifyWasm(pk, tampered, sig); err == nil {
		t.Errorf("expected verify failure on tampered data")
	}
}

func TestSign_TamperedTrustedCommentRejected(t *testing.T) {
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	data := []byte("hello")
	sig, err := Sign(sk, data, "original comment")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Mutating only the trusted comment must fail global-sig verification —
	// otherwise an attacker could relabel a sig as belonging to a
	// different file without producing a fresh signature.
	sig.TrustedComment = "trusted comment: forged"
	if err := VerifyWasm(pk, data, sig); err == nil {
		t.Errorf("expected verify failure when trusted comment is altered")
	}
}

func TestSign_TrustedCommentPrefixAutoAdded(t *testing.T) {
	sk, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	sig, err := Sign(sk, []byte("hi"), "no-prefix-here")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if !strings.HasPrefix(sig.TrustedComment, trustedCommentPrefix) {
		t.Errorf("trusted comment = %q, want prefix %q", sig.TrustedComment, trustedCommentPrefix)
	}
	// Caller that already supplied the prefix must not get it doubled.
	sig2, err := Sign(sk, []byte("hi"), trustedCommentPrefix+"already")
	if err != nil {
		t.Fatalf("sign #2: %v", err)
	}
	if strings.HasPrefix(sig2.TrustedComment, trustedCommentPrefix+trustedCommentPrefix) {
		t.Errorf("trusted comment doubled the prefix: %q", sig2.TrustedComment)
	}
}

func TestEncodeSignature_RoundTripThroughDecoder(t *testing.T) {
	sk, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	data := []byte("payload")
	sig, err := Sign(sk, data, "test")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	encoded := EncodeSignature(sig)
	decoded, err := minisign.DecodeSignature(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.KeyId != sig.KeyId {
		t.Errorf("keyId mismatch")
	}
	if decoded.Signature != sig.Signature {
		t.Errorf("signature bytes mismatch")
	}
	if decoded.GlobalSignature != sig.GlobalSignature {
		t.Errorf("global signature bytes mismatch")
	}
	if decoded.TrustedComment != sig.TrustedComment {
		t.Errorf("trusted comment mismatch: %q vs %q", decoded.TrustedComment, sig.TrustedComment)
	}
}

func TestEncodePublicKey_RoundTripThroughDecoder(t *testing.T) {
	_, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	encoded := EncodePublicKey(pk, "")
	decoded, err := minisign.DecodePublicKey(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.KeyId != pk.KeyId || decoded.PublicKey != pk.PublicKey {
		t.Errorf("decoded pubkey != original")
	}
}

func TestEncodeSecretKey_RoundTrip(t *testing.T) {
	sk, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	encoded := EncodeSecretKey(sk)
	if !strings.Contains(encoded, "platypus plugin secret key") {
		t.Errorf("secret key file missing comment marker: %q", encoded)
	}
	back, err := DecodeSecretKey(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back.KeyID != sk.KeyID {
		t.Errorf("keyID round-trip mismatch")
	}
	if string(back.Ed25519) != string(sk.Ed25519) {
		t.Errorf("private key round-trip mismatch")
	}

	// Tampered base64 must fail loudly.
	if _, err := DecodeSecretKey("comment\nnot-base64-!@#"); err == nil {
		t.Errorf("expected base64 decode failure")
	}
	// Wrong-length payload.
	short := "comment\n" + base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) + "\n"
	if _, err := DecodeSecretKey(short); err == nil {
		t.Errorf("expected length check failure")
	}
}

func TestRandomDataRoundTrip(t *testing.T) {
	// Belt-and-braces with random binary payload (extism .wasm files are
	// far from ASCII-safe).
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	sig, err := Sign(sk, data, DefaultTrustedComment("blob.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := VerifyWasm(pk, data, sig); err != nil {
		t.Errorf("verify random data: %v", err)
	}
}
