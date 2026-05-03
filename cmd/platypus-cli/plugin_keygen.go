package main

import (
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// pluginKeygenCmd writes a new ed25519 keypair as two files:
// `--out-public` is a minisign-format pubkey (deployable to agents
// under publishers/) and `--out-secret` is a Platypus-internal
// unencrypted secret key (suitable for `platypus-cli plugin sign
// --key`). The split avoids accidentally distributing the secret
// alongside the pubkey.
//
// SECRET KEY ENCRYPTION: minisign(1) encrypts secret keys with scrypt
// + a passphrase. We don't (yet) — the file is plaintext. Operators
// who need encrypted-at-rest keys should generate with `minisign -G`
// and import the pubkey via the (TBD) `import-key` subcommand.
type pluginKeygenCmd struct {
	OutSecret string `name:"out-secret" required:"" help:"Path to write the secret key file. Keep this private."`
	OutPublic string `name:"out-public" required:"" help:"Path to write the public key file (deployable to agent publishers/ dir)."`
	Force     bool   `help:"Overwrite the output files if they already exist."`
}

func (c *pluginKeygenCmd) Run(_ *runContext) error {
	if !c.Force {
		for _, p := range []string{c.OutSecret, c.OutPublic} {
			if _, err := os.Stat(p); err == nil {
				return fmt.Errorf("refusing to overwrite existing %s (pass --force to override)", p)
			}
		}
	}

	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("keygen: %w", err)
	}

	// Write secret first (mode 0600); if that fails we don't leak a
	// pubkey for which there's no recoverable secret key.
	if err := os.WriteFile(c.OutSecret, []byte(plugin.EncodeSecretKey(sk)), 0o600); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	if err := os.WriteFile(c.OutPublic, []byte(plugin.EncodePublicKey(pk, "")), 0o644); err != nil {
		// Best-effort cleanup of the orphaned secret so the operator
		// isn't left with a half-written keypair on disk.
		_ = os.Remove(c.OutSecret)
		return fmt.Errorf("write public: %w", err)
	}

	fmt.Printf("Generated keypair %s\n", plugin.HumanKeyID(pk))
	fmt.Printf("  secret: %s (mode 0600)\n", c.OutSecret)
	fmt.Printf("  public: %s\n", c.OutPublic)
	fmt.Printf("\nDistribute the public key to operators by dropping it under\n")
	fmt.Printf("  ~/.platypus/agent/plugins/publishers/%s.pub\n", plugin.HumanKeyID(pk))
	fmt.Printf("on every agent that should accept plugins signed by this key.\n")
	return nil
}
