package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes = AES-128
	plaintext := []byte("hello, platypus!")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("expected '%s', got '%s'", plaintext, decrypted)
	}
}

func TestDecryptTampered(t *testing.T) {
	key := []byte("0123456789abcdef")
	plaintext := []byte("test data")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with ciphertext
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = Decrypt(key, ciphertext)
	if err == nil {
		t.Fatal("expected error on tampered ciphertext (AES-GCM should detect)")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := []byte("0123456789abcdef")
	_, err := Decrypt(key, []byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error on too-short ciphertext")
	}
}

func TestEncryptDecryptEmpty(t *testing.T) {
	key := []byte("0123456789abcdef")
	ciphertext, err := Encrypt(key, []byte{})
	if err != nil {
		t.Fatalf("Encrypt empty failed: %v", err)
	}
	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty failed: %v", err)
	}
	if len(decrypted) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(decrypted))
	}
}
