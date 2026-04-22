package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	msg := []byte("platypus agent manifest v1.6.0")
	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(pub, msg, sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyRejectsTamperedMessage(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sig, _ := Sign(priv, []byte("original"))
	if err := Verify(pub, []byte("tampered"), sig); err == nil {
		t.Fatalf("expected verify to reject tampered message")
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	wrongPub, _, _ := ed25519.GenerateKey(rand.Reader)
	msg := []byte("msg")
	sig, _ := Sign(priv, msg)
	if err := Verify(wrongPub, msg, sig); err == nil {
		t.Fatalf("expected verify to reject wrong public key")
	}
}

func TestDecodePublicKey(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	encoded := base64.StdEncoding.EncodeToString(pub)
	got, err := DecodePublicKey(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Equal(pub) {
		t.Fatalf("decoded key does not match")
	}
}

func TestDecodePublicKeyRejectsShort(t *testing.T) {
	if _, err := DecodePublicKey(base64.StdEncoding.EncodeToString([]byte("too-short"))); err == nil {
		t.Fatalf("expected error on short key")
	}
}
