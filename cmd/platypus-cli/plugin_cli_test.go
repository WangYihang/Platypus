package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests exercise each subcommand's Run() method directly. Going
// through kong is unnecessary for exit-code coverage and would force
// us to run the binary in a subprocess.

func TestCLI_KeygenSignVerifyHappyPath(t *testing.T) {
	dir := t.TempDir()
	skPath := filepath.Join(dir, "sk")
	pkPath := filepath.Join(dir, "pk")
	wasmPath := filepath.Join(dir, "toy.wasm")
	if err := os.WriteFile(wasmPath, []byte("not really a wasm but signing is byte-agnostic"), 0o600); err != nil {
		t.Fatalf("seed wasm: %v", err)
	}

	rc := &runContext{Context: context.Background()}

	keygen := pluginKeygenCmd{OutSecret: skPath, OutPublic: pkPath}
	if err := keygen.Run(rc); err != nil {
		t.Fatalf("keygen: %v", err)
	}
	for _, p := range []string{skPath, pkPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
	}

	sign := pluginSignCmd{Key: skPath, Wasm: wasmPath}
	if err := sign.Run(rc); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := os.Stat(wasmPath + ".minisig"); err != nil {
		t.Fatalf("expected sig at %s: %v", wasmPath+".minisig", err)
	}

	verify := pluginVerifyCmd{Pub: pkPath, Wasm: wasmPath}
	if err := verify.Run(rc); err != nil {
		t.Fatalf("verify happy path: %v", err)
	}
}

func TestCLI_VerifyRejectsTamperedWasm(t *testing.T) {
	dir := t.TempDir()
	skPath := filepath.Join(dir, "sk")
	pkPath := filepath.Join(dir, "pk")
	wasmPath := filepath.Join(dir, "toy.wasm")
	if err := os.WriteFile(wasmPath, []byte("original"), 0o600); err != nil {
		t.Fatalf("seed wasm: %v", err)
	}

	rc := &runContext{Context: context.Background()}
	if err := (&pluginKeygenCmd{OutSecret: skPath, OutPublic: pkPath}).Run(rc); err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := (&pluginSignCmd{Key: skPath, Wasm: wasmPath}).Run(rc); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Tamper with the wasm AFTER signing — verification must fail.
	if err := os.WriteFile(wasmPath, []byte("tampered"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	err := (&pluginVerifyCmd{Pub: pkPath, Wasm: wasmPath}).Run(rc)
	if err == nil || !strings.Contains(err.Error(), "Invalid signature") {
		t.Errorf("expected verify failure, got %v", err)
	}
}

func TestCLI_KeygenRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	skPath := filepath.Join(dir, "sk")
	pkPath := filepath.Join(dir, "pk")
	if err := os.WriteFile(skPath, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rc := &runContext{Context: context.Background()}
	err := (&pluginKeygenCmd{OutSecret: skPath, OutPublic: pkPath}).Run(rc)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Errorf("expected refuse-overwrite, got %v", err)
	}
	// With --force, succeeds.
	if err := (&pluginKeygenCmd{OutSecret: skPath, OutPublic: pkPath, Force: true}).Run(rc); err != nil {
		t.Errorf("with --force: %v", err)
	}
}

func TestCLI_ValidateManifest(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(good, []byte(validManifestForCLI), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rc := &runContext{Context: context.Background()}
	if err := (&pluginValidateManifestCmd{File: good}).Run(rc); err != nil {
		t.Errorf("happy: %v", err)
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad,
		[]byte(strings.Replace(validManifestForCLI, "version: 1.0.0", "version: 1.0", 1)), 0o600); err != nil {
		t.Fatalf("seed bad: %v", err)
	}
	err := (&pluginValidateManifestCmd{File: bad}).Run(rc)
	if err == nil || !strings.Contains(err.Error(), "strict semver") {
		t.Errorf("expected semver error, got %v", err)
	}

	// Missing file.
	if err := (&pluginValidateManifestCmd{File: filepath.Join(dir, "missing.yaml")}).Run(rc); err == nil {
		t.Errorf("expected error for missing file")
	}
}

const validManifestForCLI = `
api_version: 1
id: com.example.demo
name: Demo
version: 1.0.0
author: { name: Test }
runtime:
  type: wasm
  entry: demo.wasm
  abi: extism/1
rpc:
  - name: ping
    request:  { proto: P }
    response: { proto: R }
capabilities:
  kv: true
resources:
  max_memory_mb: 16
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: ABC
  sig_file: demo.wasm.minisig
`
