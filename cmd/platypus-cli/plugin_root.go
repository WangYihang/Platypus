package main

// pluginCmd is the `plugin` subcommand group. Each field is a
// leaf subcommand; kong uses the `cmd:""` tag and the field name (in
// snake-case) to wire up CLI routing. Keep one file per leaf so the
// command set stays grep-able.
type pluginCmd struct {
	Keygen          pluginKeygenCmd          `cmd:"" help:"Generate a fresh ed25519 publisher keypair (minisign-compatible pubkey + Platypus secret-key file)."`
	Sign            pluginSignCmd            `cmd:"" help:"Sign a .wasm artefact with a publisher secret key, producing a detached .minisig file."`
	Verify          pluginVerifyCmd          `cmd:"" help:"Verify a .wasm against a publisher pubkey + .minisig file."`
	ValidateManifest pluginValidateManifestCmd `cmd:"" name:"validate-manifest" help:"Parse and validate a plugin.yaml without signing or installing."`
}
