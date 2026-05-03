package main

import (
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// pluginVerifyCmd is the standalone counterpart to the agent's
// verify-on-load step. Useful for plugin authors to confirm their
// signing flow works before pushing to the marketplace, and for
// operators auditing a third-party plugin out-of-band.
type pluginVerifyCmd struct {
	Pub  string `required:"" help:"Path to the publisher's public key file (minisign-format .pub)."`
	Wasm string `required:"" help:"Path to the .wasm artefact whose signature is being verified."`
	Sig  string `help:"Path to the .minisig file. Defaults to <wasm>.minisig."`
}

func (c *pluginVerifyCmd) Run(_ *runContext) error {
	pk, keyID, err := plugin.LoadPublicKey(c.Pub)
	if err != nil {
		return err
	}
	sigPath := c.Sig
	if sigPath == "" {
		sigPath = c.Wasm + ".minisig"
	}
	if err := plugin.VerifyWasmFile(pk, c.Wasm, sigPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "OK: %s verified against publisher %s\n", c.Wasm, keyID)
	return nil
}
