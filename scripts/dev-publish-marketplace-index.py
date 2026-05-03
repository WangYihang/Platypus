#!/usr/bin/env python3
"""Generate a marketplace index.json from a staged plugin bundle.

Used by the agent-publisher compose sidecar. Walks
$BUNDLE_ROOT/plugins/<plugin_id>/<version>/ for each plugin's
plugin.yaml + .wasm + .minisig, computes the wasm sha256, embeds the
publisher .pub (base64-encoded) into every entry's
publisher_pubkey_b64 field, and emits the result on stdout.

Required env vars:
  BUNDLE_ROOT          — the staged tree on the publisher's local fs
  SERVER_BUNDLE_PATH   — the SERVER's view of the same tree (used to
                         build file:// URLs in the index — the server
                         mounts platypus_data at a different path than
                         the publisher does, so the URL must point at
                         the server's path)
  PUBKEY_FILE          — path to the publisher's .pub file

The server's catalog refresh + install_marketplace artefact fetch both
understand file:// URLs, so the operator never needs to stand up a
separate HTTP server inside the compose stack.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import sys
import time

import yaml

BUNDLE_ROOT = os.environ["BUNDLE_ROOT"]
SERVER_BUNDLE_PATH = os.environ["SERVER_BUNDLE_PATH"]
PUBKEY_FILE = os.environ["PUBKEY_FILE"]

with open(PUBKEY_FILE, "rb") as f:
    pubkey_b64 = base64.b64encode(f.read()).decode()

plugins_root = os.path.join(BUNDLE_ROOT, "plugins")
plugins: list[dict] = []
for pid in sorted(os.listdir(plugins_root)):
    pid_dir = os.path.join(plugins_root, pid)
    for ver in sorted(os.listdir(pid_dir)):
        d = os.path.join(pid_dir, ver)
        manifest_path = os.path.join(d, "plugin.yaml")
        if not os.path.isfile(manifest_path):
            continue
        with open(manifest_path) as f:
            m = yaml.safe_load(f)
        # Cargo names the .wasm after the [lib] name, not the dir; glob it.
        wasm_files = [x for x in os.listdir(d) if x.endswith(".wasm")]
        if len(wasm_files) != 1:
            print(
                f"skipping {pid}@{ver}: expected exactly one .wasm, "
                f"found {len(wasm_files)}",
                file=sys.stderr,
            )
            continue
        wasm = wasm_files[0]
        with open(os.path.join(d, wasm), "rb") as f:
            sha = hashlib.sha256(f.read()).hexdigest()
        author_field = m.get("author") or {}
        author_name = (
            author_field.get("name", "") if isinstance(author_field, dict) else str(author_field)
        )
        # file:// URLs use the SERVER's mount path. The publisher's
        # local view is /output/plugin-marketplace; the server reads
        # /app/data/plugin-marketplace. The catalog refresh + the
        # artefact fetcher both run on the server side, so all URLs
        # must speak its path.
        server_dir = f"file://{SERVER_BUNDLE_PATH}/plugins/{pid}/{ver}"
        plugins.append(
            {
                "plugin_id": pid,
                "version": ver,
                "name": m.get("name", pid),
                "author": author_name,
                "license": m.get("license", ""),
                "homepage": m.get("homepage", ""),
                "description": (m.get("description") or "").strip(),
                "latest_version": ver,
                "publisher_key_id": (m.get("signature") or {}).get("key_id", ""),
                "wasm_url": f"{server_dir}/{wasm}",
                "signature_url": f"{server_dir}/{wasm}.minisig",
                "manifest_url": f"{server_dir}/plugin.yaml",
                "wasm_sha256_hex": sha,
                "publisher_pubkey_b64": pubkey_b64,
                "capabilities": list((m.get("capabilities") or {}).keys()),
            }
        )

print(
    json.dumps(
        {"generated_at_unix": int(time.time()), "plugins": plugins},
        indent=2,
    )
)
