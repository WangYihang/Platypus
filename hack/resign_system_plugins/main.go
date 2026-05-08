// resign_system_plugins re-signs every staged plugin .wasm under
// internal/server/sysplugins/embedded/system-plugins/<id>/<ver>/
// with the current hack/.system-signing.secret. Unlike
// stage_system_plugins it does NOT rebuild wasm bytes — it just
// mints fresh .minisig files matching the current publisher.pub
// key id, and patches each staged plugin.yaml's signature.key_id
// so the manifest view + sig key id read coherently.
//
// Use case: the staged tree's signatures drifted (the active
// signing secret was rotated but only some plugins were re-staged),
// and you want to bring every embedded sig back in sync without
// the rust + tinygo build cost of stage-system-plugins.
//
// Run from the repo root:
//
//	go run ./hack/resign_system_plugins
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	plugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

const (
	stagedRoot = "internal/server/sysplugins/embedded/system-plugins"
	secretPath = "hack/.system-signing.secret"
)

var keyIDRe = regexp.MustCompile(`(?m)^(\s*key_id:\s*)\S+`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "resign:", err)
		os.Exit(1)
	}
}

func run() error {
	skBytes, err := os.ReadFile(secretPath)
	if err != nil {
		return fmt.Errorf("read secret: %w", err)
	}
	sk, err := plugin.DecodeSecretKey(string(skBytes))
	if err != nil {
		return fmt.Errorf("decode secret: %w", err)
	}
	keyID := fmt.Sprintf("%016X", binary.LittleEndian.Uint64(sk.KeyID[:]))

	walked := 0
	signed := 0
	err = filepath.WalkDir(stagedRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".minisig") {
			return nil
		}
		walked++
		wasmPath := strings.TrimSuffix(path, ".minisig")
		wasmBytes, rdErr := os.ReadFile(wasmPath)
		if rdErr != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", path, rdErr)
			return nil
		}
		// Derive the trusted comment from the directory layout:
		// sysplugins/<id>/<ver>/<entry>.wasm.minisig.
		rel, _ := filepath.Rel(stagedRoot, wasmPath)
		parts := strings.Split(rel, string(filepath.Separator))
		comment := "platypus plugin signature"
		if len(parts) >= 2 {
			comment = fmt.Sprintf("%s@%s", parts[0], parts[1])
			// Patch the sibling plugin.yaml's signature.key_id so
			// audit views render a coherent (manifest, sig) pair.
			yamlPath := filepath.Join(filepath.Dir(wasmPath), "plugin.yaml")
			if raw, yErr := os.ReadFile(yamlPath); yErr == nil {
				patched := keyIDRe.ReplaceAllString(string(raw), "${1}"+keyID)
				if patched != string(raw) {
					_ = os.WriteFile(yamlPath, []byte(patched), 0o644)
				}
			}
		}
		sig, sErr := plugin.Sign(sk, wasmBytes, comment)
		if sErr != nil {
			return fmt.Errorf("sign %s: %w", wasmPath, sErr)
		}
		if wErr := os.WriteFile(path, []byte(plugin.EncodeSignature(sig)), 0o644); wErr != nil {
			return wErr
		}
		signed++
		fmt.Printf("  resigned %s\n", rel)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("resigned %d / %d minisig files (key %s)\n", signed, walked, keyID)
	return nil
}
