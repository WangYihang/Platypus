# Platypus Desktop + standalone Web UI

Two UI flavours share this React codebase:

- **Desktop** (Wails v2) — native window, OS-keychain-backed credentials,
  `desktop/build/bin/<name>` after `make desktop-build`. Install once, use
  anywhere.
- **Standalone web UI** — same pages, built into a static bundle
  (`desktop/frontend/dist-web/`) you can host anywhere. Useful for quick
  feature verification without installing the native app, or for remoting
  into a headless ops box.

Both point at the same `platypus-server` REST+WS API; the server ships no
embedded UI and doesn't know about either frontend.

## Architecture

| Layer | Tech | Notes |
|---|---|---|
| Shell | Wails v2 / browser | Native WKWebView/webkit2gtk/WebView2 or any browser |
| UI | React 18 + Vite + TypeScript + antd 6 + xterm.js | |
| Glue (desktop) | Wails JS↔Go bindings | Auto-generated under `frontend/wailsjs/`; Go owns HTTP/WS/keychain/file IO |
| Glue (web) | `frontend/src/platform/*.web.ts` | Drop-in shim — same names as the Wails bindings, backed by `fetch` + browser WebSocket. Credentials cached in `localStorage`. |

Vite picks the glue at build time: default mode → Wails bindings, `--mode web`
aliases the three wailsjs imports (`go/app/App`, `runtime/runtime`, `go/models`)
to the shims under `src/platform/`. Every page under `src/pages/` is shared
verbatim across both modes.

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
│   │       ├── Listeners.tsx     # CRUD agent ingress listeners
│   │       ├── Files.tsx         # Chunked upload/download
│   │       └── Tunnels.tsx       # pull/push/dynamic SOCKS5/internet
│   ├── wailsjs/                  # auto-generated; gitignored
│   └── package.json
└── wails.json
```

## Develop

Desktop (Wails):

```bash
make desktop-deps         # one-time: install Wails CLI + pnpm packages
make desktop-bindings     # any time you add/remove App methods on the Go side
make desktop-dev          # hot-reload dev mode
```

Web UI:

```bash
make web-ui               # builds desktop/frontend/dist-web/
make web-ui-serve         # serves dist-web/ at http://localhost:8080
```

Any static host works — GitHub Pages, Netlify, S3, nginx. CORS on
platypus-server is `*`, so cross-origin fetch + WebSocket work out of the
box. Only requirement: UI and server must share URL scheme (both HTTP
locally, both HTTPS in prod).

On Linux, install the WebKit/GTK dev libs first:

```bash
sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev pkg-config build-essential
```

The Makefile passes `-tags webkit2_41` so Wails uses the webkit2gtk-4.1 bindings
(the only version shipped on Ubuntu 22.04+ / Fedora 37+ / Debian 12+). Override
via `make desktop-build WAILS_TAGS=""` if you're on an older distro that still
ships webkit2gtk-4.0. macOS / Windows ignore the tag.

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
2. Pick a client:
   - **Desktop**: launch the native app, click **Add Profile**, paste
     URL + secret, **Connect**. Secrets live in the OS keychain (Keychain
     on macOS, Credential Vault on Windows, Secret Service on Linux).
   - **Web UI**: open `http://localhost:8080` (from `make web-ui-serve`),
     paste URL + secret, **Connect**. URL + bearer token cache to
     `localStorage`; Disconnect clears both.
3. Install and run the agent on a managed host:
   `curl -fsSL http://<server>:13339/agent/<server>:13337 | sh`
4. The session appears in the Sessions tab; click **Open Terminal** to
   interact, or use **Group**+**Dispatch Command** to fan out a shell
   command to multiple sessions.

## Out of MVP scope

- Real-time `/notify` push events in the web UI (new-session toast, upgrade
  progress bar). Desktop has this via Wails events; web mode currently
  relies on a Refresh button.
- Directory listing in the file browser (server only exposes read/write/size).
- WebGL renderer for xterm (Wails WebView compatibility varies across
  platforms — using Canvas).
- Zmodem file transfer.
- OpenAPI-codegen desktop client. Server now ships Swagger at
  `/swagger/index.html` (regenerated via `make swag`); switching the
  desktop HTTP layer to `oapi-codegen` is ~300-500 lines of churn and
  was deferred.
