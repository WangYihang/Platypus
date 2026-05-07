// stage_releases reads goreleaser's dist/artifacts.json, lays the
// platypus-agent binaries out under <releases-dir>/artifacts/<version>/
// <os>/<arch>/, builds the signed release manifest at
// <releases-dir>/manifest/<channel>.json{,.sig}, and exits.
//
// Run via the Makefile's `releases` target — `goreleaser build` is
// expected to have run first so `dist/artifacts.json` is present. The
// privkey path points at a PEM-encoded Ed25519 secret key (the dev
// keypair the Makefile mints under hack/ on first run).
//
// The output schema mirrors internal/core/distributor.go's Manifest /
// ManifestArtifact types so the server's existing parser eats it
// without modification.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type goreleaserArtifact struct {
	Path   string `json:"path"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Type   string `json:"type"`
	Extra  struct {
		ID  string `json:"ID"`
		Ext string `json:"Ext"`
	} `json:"extra"`
}

type manifestArtifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	Version    string             `json:"version"`
	Channel    string             `json:"channel"`
	ReleasedAt time.Time          `json:"released_at"`
	Artifacts  []manifestArtifact `json:"artifacts"`
}

func main() {
	var (
		distDir     = flag.String("dist", "dist", "goreleaser dist directory")
		releasesDir = flag.String("releases-dir", "data/releases", "output root (LocalStore prefix)")
		version     = flag.String("version", "0.0.0-dev", "release version")
		channel     = flag.String("channel", "stable", "release channel")
		privkeyPath = flag.String("privkey", "hack/.agent-signing.pem", "PEM-encoded Ed25519 private key for manifest signing")
	)
	flag.Parse()

	if err := run(*distDir, *releasesDir, *version, *channel, *privkeyPath); err != nil {
		fmt.Fprintln(os.Stderr, "stage_releases:", err)
		os.Exit(1)
	}
}

func run(distDir, releasesDir, version, channel, privkeyPath string) error {
	priv, err := loadEd25519PEM(privkeyPath)
	if err != nil {
		return fmt.Errorf("load privkey: %w", err)
	}

	raw, err := os.ReadFile(filepath.Join(distDir, "artifacts.json"))
	if err != nil {
		return fmt.Errorf("read goreleaser artifacts.json: %w", err)
	}
	var grArts []goreleaserArtifact
	if err := json.Unmarshal(raw, &grArts); err != nil {
		return fmt.Errorf("parse goreleaser artifacts.json: %w", err)
	}

	var entries []manifestArtifact
	for _, a := range grArts {
		// Filter to platypus-agent Binary entries; goreleaser also
		// emits Metadata + (when configured) Archive / Checksum rows.
		if a.Type != "Binary" || a.Extra.ID != "platypus-agent" {
			continue
		}
		basename := "platypus-agent" + a.Extra.Ext
		key := fmt.Sprintf("artifacts/%s/%s/%s/%s", version, a.GOOS, a.GOARCH, basename)
		dst := filepath.Join(releasesDir, key)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		size, sum, err := copyAndDigest(a.Path, dst)
		if err != nil {
			return fmt.Errorf("copy %s/%s: %w", a.GOOS, a.GOARCH, err)
		}
		entries = append(entries, manifestArtifact{
			OS:     a.GOOS,
			Arch:   a.GOARCH,
			Key:    key,
			Size:   size,
			SHA256: sum,
		})
		fmt.Printf("→ staged %s/%s (%d bytes)\n", a.GOOS, a.GOARCH, size)
	}
	if len(entries) == 0 {
		return fmt.Errorf("no platypus-agent Binary entries in %s — did `goreleaser build --id platypus-agent` succeed?", distDir)
	}

	man := manifest{
		Version:    version,
		Channel:    channel,
		ReleasedAt: time.Now().UTC().Truncate(time.Second),
		Artifacts:  entries,
	}
	manBytes, err := json.Marshal(man)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, manBytes)

	manifestDir := filepath.Join(releasesDir, "manifest")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return err
	}
	manPath := filepath.Join(manifestDir, channel+".json")
	if err := os.WriteFile(manPath, manBytes, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(manPath+".sig", sig, 0o644); err != nil {
		return err
	}
	fmt.Printf("→ wrote %s (%d artefacts)\n", manPath, len(entries))
	return nil
}

func copyAndDigest(src, dst string) (int64, string, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), in)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// loadEd25519PEM parses a PKCS#8 PEM-encoded Ed25519 private key —
// the format `openssl genpkey -algorithm ED25519` writes by default.
func loadEd25519PEM(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM block found", path)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ed, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: not an Ed25519 key (got %T)", path, key)
	}
	return ed, nil
}
