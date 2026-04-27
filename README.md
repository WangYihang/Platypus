# Platypus

[![GitHub stars](https://img.shields.io/github/stars/WangYihang/Platypus.svg)](https://github.com/WangYihang/Platypus/stargazers)
[![GitHub license](https://img.shields.io/github/license/WangYihang/Platypus.svg)](https://github.com/WangYihang/Platypus)
[![GitHub Release Downloads](https://img.shields.io/github/downloads/wangyihang/platypus/total)](https://github.com/WangYihang/Platypus/releases)
[![Sponsors](https://opencollective.com/platypus/tiers/badge.svg)](https://opencollective.com/platypus)

A host management hub for fleets of Linux machines. Install the Platypus
agent on every host you own; the agent dials back to your Platypus server
over TLS + protobuf; from the server you get an interactive shell, file
management, and network tunnelling on every managed host — through one
central control plane.

## Screenshots

A Vercel-style flat-nav UI with a project dashboard, a host/listener/session
browser, a multi-host dispatch console, and an admin user manager. See the
full gallery at [`docs/screenshots/`](docs/screenshots/README.md). Re-run
`make screenshots` to regenerate them from the live app.

[![Project overview dashboard](docs/screenshots/04-project-overview.png)](docs/screenshots/README.md)

## Architecture

Platypus ships as two backend binaries plus a standalone desktop client:

| Binary | Role |
|---|---|
| `platypus-server`  | Daemon. Accepts inbound agent connections on TLS ingress ports; exposes a REST + WebSocket API for admin tooling; serves agent binaries for distribution. |
| `platypus-agent`   | The process that runs on each managed host. Dials back to the server over TLS + protobuf. |
| `platypus-desktop` | Native (Wails v2) desktop GUI. Connect to any reachable server with URL + secret; tabbed UI for sessions, terminals, listeners, files, tunnels. See [`desktop/`](./desktop/). |

The server is purely an API — no embedded web UI. Multiple desktops can
connect to the same server simultaneously.

## Features

- TLS + protobuf channel between agent and server
- Multiple ingress ports so fleets in different networks share one hub
- Interactive shell per host, streamed over WebSocket
- File read / write / upload / download with chunked transfer
- Network tunnels: local-port-forward, remote-port-forward, dynamic SOCKS5
- Bearer-token-authenticated [REST API](./docs/RESTful.md)
- [Python SDK](https://github.com/WangYihang/Platypus-Python)
- Auto-start listeners from `config.yml`
- Graceful shutdown (drains connections on SIGINT/SIGTERM within 30s)

## Documents

* [Chinese | 中文文档](https://platypus-reverse-shell.vercel.app/)

## Quick start

### One-Click Deployment (Recommended)

The fastest way to get a full Platypus system running (including the Server, Web UI, and a MinIO-backed Agent Distributor) is using Docker Compose:

```bash
git clone https://github.com/WangYihang/Platypus
cd Platypus
docker-compose up -d
```

*   **Web UI**: Open `https://localhost:9443` in your browser (the
    container ships with a self-signed cert; your browser will prompt
    you to accept it on first visit).
*   **Login**: The `bootstrap_secret` is written to
    `/app/data/bootstrap.secret` inside the container on first boot
    (mode 0600). Read it with
    `docker compose exec platypus-server cat /app/data/bootstrap.secret`,
    use it once to create the first admin, then delete the file. After
    bootstrap completes the secret is no longer accepted.
*   **Add Agents**: Click "Add Agent" in the UI to get a one-line
    `curl ... | sh` command, signed and pinned to the project CA, to
    deploy agents to your fleet.

### Build from source

Requires Go 1.24+ and `protoc` (only if you regenerate protobuf code).

```bash
git clone https://github.com/WangYihang/Platypus
cd Platypus
make build              # → ./build/{platypus-server,platypus-agent}
```

Other useful targets: `make test`, `make lint`, `make snapshot` (cross-platform via goreleaser), `make help`.

### Development (pre-commit hooks)

Contributors should install the git hooks so `gofmt` / `goimports` / `go vet` /
`golangci-lint` run before each commit:

```bash
pip install pre-commit   # or: pipx install pre-commit
make hooks               # one-time: wires .git/hooks/pre-commit
make pre-commit          # optional: run all hooks against every file now
```

### Build the desktop app

Requires Node 22+, Wails CLI dependencies (`wails doctor`), and the platform's WebView libraries (webkit2gtk-4.1 on Linux, WebView2 on Windows, WKWebView on macOS).

```bash
make desktop-deps       # one-time: install Wails CLI + pnpm packages
make desktop-build      # → desktop/build/bin/platypus-desktop
make desktop-dev        # hot-reload dev mode
```

### Or: use the standalone web UI (no install)

Same pages, same features (minus real-time event push), runs in any browser:

```bash
make web-ui             # → desktop/frontend/dist-web/ (static bundle)
make web-ui-serve       # preview at http://localhost:8080
```

`dist-web/` is fully static; drop it on GitHub Pages / S3 / nginx. Point it at any `platypus-server` via the login form.

Full notes in [`desktop/README.md`](./desktop/README.md).

### Install from release binaries

Download the appropriate archive for your OS/arch from the [Releases page](https://github.com/WangYihang/Platypus/releases), extract, and run.

### Run with Docker

```bash
docker build -t platypus-server .
docker run --rm -p 9443:9443 \
  -e PLATYPUS_DEV=1 \
  -v $(pwd)/config.yml:/config.yml \
  platypus-server
```

`PLATYPUS_DEV=1` enables the on-disk KEK fallback so a fresh start
"Just Works"; for production, drop that env var and set
`PLATYPUS_CA_KEK` (32 bytes hex; `openssl rand -hex 32`) instead so
the CA private-key encryption key is never written next to the
encrypted database.

### Run

```bash
./build/platypus-server                # foreground; Ctrl-C for graceful shutdown
```

For production, run the server under `systemd` rather than backgrounding it manually.

## Usage

### Topology

* Server: `192.168.88.129`
  * Unified TLS ingress (REST API + agent link + installer): `0.0.0.0:9443`
* Managed host: `192.168.88.130` (runs `platypus-agent`)

### Quick tour

First, run `./platypus-server`. A `config.yml` is generated from
`assets/config.example.yml` if none exists. Defaults are sensible.

To enrol a new managed host:

1. Open the Web UI / desktop app, navigate to the project, click
   **Add Agent**.
2. Pick the target OS / arch and TTL, click **Generate**.
3. Copy the one-liner the UI displays and run it on the managed host.

The server-rendered one-liner looks like:

```bash
curl -fsSL https://<server>:9443/api/v1/install/<one-shot-token> | sh
```

The single-use install token expires shortly after issuance and is
burnt on the first successful hit. The script the server returns:

* embeds the project CA so the agent binary download is verified with
  `curl --cacert` (no `-k`, no TOFU);
* injects a single-use PAT so the agent can complete enrolment and
  obtain its mTLS client certificate;
* refuses to run if the server has no project CA configured, unless
  the operator explicitly sets `PLATYPUS_INSECURE_DOWNLOAD=1`.

> **Do not** hand-write `curl http://...` URLs against the legacy
> `/agent/<host>:<port>` path — that flow is removed. Always render
> the install command via the UI so it carries the trust anchor.

### Desktop / Web

Open the desktop app (`platypus-desktop`) or the web UI (`make web-ui-serve`),
fill in the server URL and secret, and you're in. Tabs: Sessions (every
agent that's dialled in), Terminal, Files, Tunnels, Listeners.

## Advanced usages

* [REST API](./docs/RESTful.md)
* Python SDK

## Other materials

* [Presentation on KCon 2019](https://github.com/WangYihang/Presentations/blob/master/2019-08-24%20Introduction%20to%20Platypus%20(KCon)/Introduction%20to%20Platypus%20on%20KCon%202019.pdf)
* [Presentation on GCSIS 2021](https://github.com/WangYihang/Presentations/blob/master/2021-04-24%20Introduction%20to%20Platypus%20(GCSIS)/Introduction%20to%20Platypus%20on%20GCSIS%202021.pptx)

## Contributors

This project exists thanks to all the people who contribute.
<a href="https://github.com/WangYihang/Platypus/graphs/contributors"><img src="https://opencollective.com/Platypus/contributors.svg?width=890&button=false" /></a>

## Backers

Thank you to all our backers! 🙏 [[Become a backer](https://opencollective.com/Platypus#backer)]

<a href="https://opencollective.com/Platypus#backers" target="_blank"><img src="https://opencollective.com/Platypus/backers.svg?width=890"></a>

## Sponsors

Support this project by becoming a sponsor. Your logo will show up here with a link to your website. [[Become a sponsor](https://opencollective.com/Platypus#sponsor)]

<a href="https://opencollective.com/Platypus/sponsor/0/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/0/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/1/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/1/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/2/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/2/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/3/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/3/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/4/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/4/avatar.svg"></a>
