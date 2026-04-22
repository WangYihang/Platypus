package mesh

import (
	"crypto/ed25519"
	"os"
)

// signBytes is a small wrapper kept separate so tests can stub it.
func signBytes(priv ed25519.PrivateKey, msg []byte) []byte {
	return ed25519.Sign(priv, msg)
}

// userHomeDir wraps os.UserHomeDir so tests can override it if needed.
var userHomeDir = func() (string, error) {
	return os.UserHomeDir()
}
