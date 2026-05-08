// stage_system_plugins is a one-shot helper used to populate
// internal/server/sysplugins/embedded/ from the rust artefacts under
// example/plugins/system/. Run via:
//
//	go run ./hack/stage_system_plugins
//
// from the repo root, AFTER `cargo build --release --target
// wasm32-unknown-unknown` in each plugin directory.
//
// On every run it:
//  1. Reads (or mints) the system signing keypair at
//     hack/.system-signing.secret. The corresponding pubkey is staged
//     at internal/server/sysplugins/embedded/system-plugins/publisher.pub.
//     The secret is gitignored — losing it just means the next run
//     mints a new one and re-signs every plugin.
//  2. For each plugin under example/plugins/system/<dir>/:
//     - rewrites its plugin.yaml signature.key_id to match the
//     system key,
//     - locates target/wasm32-unknown-unknown/release/<entry>.wasm,
//     - signs it,
//     - copies plugin.yaml + .wasm + .minisig into
//     internal/server/sysplugins/embedded/system-plugins/<plugin-id>/<version>/.
//
// The staged tree is what go:embed pulls into the server binary. The
// server's reconciler falls back to it when <data-dir>/system-plugins/
// is empty.
package main

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/jedisct1/go-minisign"
	"gopkg.in/yaml.v3"

	plugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

const (
	pluginsRoot   = "example/plugins/system"
	stagedRoot    = "internal/server/sysplugins/embedded/system-plugins"
	secretPath    = "hack/.system-signing.secret"
	publisherName = "untrusted comment: Platypus system publisher"
)

type manifestSnippet struct {
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
	Runtime struct {
		Entry string `yaml:"entry"`
	} `yaml:"runtime"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "stage_system_plugins:", err)
		os.Exit(1)
	}
}

func run() error {
	sk, pk, err := loadOrMintKey()
	if err != nil {
		return fmt.Errorf("keypair: %w", err)
	}
	keyID := fmt.Sprintf("%016X", binary.LittleEndian.Uint64(pk.KeyId[:]))

	if err := os.MkdirAll(stagedRoot, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stagedRoot, "publisher.pub"),
		[]byte(plugin.EncodePublicKey(pk, publisherName)), 0o644); err != nil {
		return fmt.Errorf("write publisher.pub: %w", err)
	}

	plugins, err := os.ReadDir(pluginsRoot)
	if err != nil {
		return fmt.Errorf("read %s: %w", pluginsRoot, err)
	}
	staged := 0
	for _, d := range plugins {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(pluginsRoot, d.Name())
		manifestPath := filepath.Join(dir, "plugin.yaml")
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", d.Name(), err)
			continue
		}
		var ms manifestSnippet
		if err := yaml.Unmarshal(raw, &ms); err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", d.Name(), err)
			continue
		}
		if ms.ID == "" || ms.Version == "" || ms.Runtime.Entry == "" {
			fmt.Fprintf(os.Stderr, "  skip %s: incomplete manifest\n", d.Name())
			continue
		}
		wasmPath := filepath.Join(dir, "target/wasm32-unknown-unknown/release", ms.Runtime.Entry)
		wasmBytes, err := os.ReadFile(wasmPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: build it first (%v)\n", d.Name(), err)
			continue
		}

		stagedDir := filepath.Join(stagedRoot, ms.ID, ms.Version)
		if err := os.MkdirAll(stagedDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stagedDir, "plugin.yaml"),
			[]byte(rewriteKeyID(string(raw), keyID)), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stagedDir, ms.Runtime.Entry),
			wasmBytes, 0o644); err != nil {
			return err
		}
		sig, err := plugin.Sign(sk, wasmBytes,
			fmt.Sprintf("%s@%s", ms.ID, ms.Version))
		if err != nil {
			return fmt.Errorf("sign %s: %w", ms.ID, err)
		}
		if err := os.WriteFile(filepath.Join(stagedDir, ms.Runtime.Entry+".minisig"),
			[]byte(plugin.EncodeSignature(sig)), 0o644); err != nil {
			return err
		}
		fmt.Printf("  staged %s@%s -> %s\n", ms.ID, ms.Version, stagedDir)
		staged++
	}
	fmt.Printf("staged %d plugin(s) under %s with key %s\n", staged, stagedRoot, keyID)
	return nil
}

func loadOrMintKey() (plugin.SecretKey, minisign.PublicKey, error) {
	if data, err := os.ReadFile(secretPath); err == nil {
		sk, err := plugin.DecodeSecretKey(string(data))
		if err == nil {
			return sk, pkFromSk(sk), nil
		}
		fmt.Fprintf(os.Stderr, "warning: %s unreadable (%v), regenerating\n", secretPath, err)
	}
	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		return plugin.SecretKey{}, minisign.PublicKey{}, err
	}
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o755); err != nil {
		return plugin.SecretKey{}, minisign.PublicKey{}, err
	}
	if err := os.WriteFile(secretPath, []byte(plugin.EncodeSecretKey(sk)), 0o600); err != nil {
		return plugin.SecretKey{}, minisign.PublicKey{}, err
	}
	return sk, pk, nil
}

// pkFromSk reconstructs the matching public key from a SecretKey.
// Ed25519 private keys carry the public bytes in their second half
// (per ed25519.PrivateKey docs).
func pkFromSk(sk plugin.SecretKey) minisign.PublicKey {
	var pk minisign.PublicKey
	pk.SignatureAlgorithm = [2]byte{'E', 'd'}
	pk.KeyId = sk.KeyID
	pubBytes := sk.Ed25519[ed25519.PrivateKeySize-ed25519.PublicKeySize:]
	copy(pk.PublicKey[:], pubBytes)
	return pk
}

// rewriteKeyID swaps in the active system key id without disturbing
// the rest of the manifest formatting (preserves comments, blank
// lines that yaml.Marshal would drop).
func rewriteKeyID(src, keyID string) string {
	re := regexp.MustCompile(`(?m)^(\s*key_id:\s*)\S+`)
	return re.ReplaceAllString(src, "${1}"+keyID)
}
