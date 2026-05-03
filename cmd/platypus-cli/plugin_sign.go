package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// pluginSignCmd produces a detached .minisig over a .wasm artefact.
// Output defaults to <wasm>.minisig next to the input so the
// extracted directory layout (`plugin.yaml`, `<entry>.wasm`,
// `<entry>.wasm.minisig`) drops into place without renames.
type pluginSignCmd struct {
	Key     string `required:"" help:"Path to the secret key file produced by 'platypus-cli plugin keygen'."`
	Wasm    string `required:"" help:"Path to the .wasm artefact to sign."`
	Out     string `help:"Path to write the .minisig file. Defaults to <wasm>.minisig."`
	Comment string `help:"Trusted-comment text baked into the signature. Defaults to a timestamp + filename."`
	Force   bool   `help:"Overwrite the output file if it already exists."`
}

func (c *pluginSignCmd) Run(_ *runContext) error {
	skBytes, err := os.ReadFile(c.Key)
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}
	sk, err := plugin.DecodeSecretKey(string(skBytes))
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}

	wasm, err := os.ReadFile(c.Wasm)
	if err != nil {
		return fmt.Errorf("read wasm: %w", err)
	}

	out := c.Out
	if out == "" {
		out = c.Wasm + ".minisig"
	}
	if !c.Force {
		if _, err := os.Stat(out); err == nil {
			return fmt.Errorf("refusing to overwrite existing %s (pass --force to override)", out)
		}
	}

	comment := c.Comment
	if comment == "" {
		comment = plugin.DefaultTrustedComment(filepath.Base(c.Wasm))
	}

	sig, err := plugin.Sign(sk, wasm, comment)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	if err := os.WriteFile(out, []byte(plugin.EncodeSignature(sig)), 0o644); err != nil {
		return fmt.Errorf("write sig: %w", err)
	}
	fmt.Printf("Signed %s -> %s\n", c.Wasm, out)
	return nil
}
