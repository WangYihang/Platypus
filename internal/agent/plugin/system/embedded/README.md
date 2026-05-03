# System plugins

This directory holds the system plugins shipped inside the
platypus-agent binary. The build pipeline embeds the entire tree via
`//go:embed all:embedded` (see `embed.go` next to this file); on every
agent boot, `system.EnsureInstalled` walks the FS and auto-installs
each `<plugin_id>/<version>/` bundle it finds.

## Layout

```
embedded/
  publisher.pub                       minisign-format system signing pubkey
  <plugin_id>/<version>/
    plugin.yaml
    <entry>.wasm
    <entry>.wasm.minisig
```

`publisher.pub` is the Ed25519 public key paired with the system
signing secret. The signing secret lives outside this repo; only the
release pipeline (or a maintainer building from source) can produce
new system-plugin bundles.

## Adding a system plugin

1. Build the plugin (see `docs/plugins/AUTHORS.md` for the toolchain).
2. Sign the produced `.wasm` with the system signing secret:
   ```
   platypus-cli plugin sign --key sys.platypus --wasm foo.wasm
   ```
3. Stage the manifest, wasm, and `.minisig` under
   `embedded/<plugin_id>/<version>/`.
4. Rebuild the agent. On next boot, every connected agent picks up
   the new plugin automatically.

## Empty for now

This directory currently contains only this README and `publisher.pub`
(once the signing key is generated). Real system plugins land in
follow-up commits as each built-in handler is migrated to the plugin
runtime.
