# RESTful API

Platypus exposes a bearer-token-authenticated REST + WebSocket API for
managing agent listeners, agent sessions, file transfer, and network
tunnels. Swagger docs live at `http://<server>:7331/swagger/index.html`
after startup; the canonical schema is generated from the `//@` annotations
on the Go handlers via `make swag`.

Quick reference:

## Auth

* `POST /api/v1/auth/token` — exchange `{secret}` for a bearer token.
* `POST /api/v1/ws/ticket` — mint a one-shot short-lived ticket for
  browser WebSocket upgrades.

## Listeners (TLS ingress ports)

* `GET  /api/v1/listeners`          — list every TLS ingress
* `GET  /api/v1/listeners/:id`      — fetch one ingress
* `POST /api/v1/listeners`          — open a new ingress `{host, port}`
* `DELETE /api/v1/listeners/:id`    — stop an ingress
* `GET  /api/v1/listeners/:id/sessions` — agents dialled in via this ingress

## Sessions (connected agents)

* `GET  /api/v1/sessions`                 — list every connected agent
* `GET  /api/v1/sessions/:id`             — fetch one agent session
* `PATCH /api/v1/sessions/:id`            — update alias / group_dispatch
* `DELETE /api/v1/sessions/:id`           — disconnect an agent
* `POST /api/v1/sessions/:id/exec`        — run a single command `{command}`
* `POST /api/v1/sessions/:id/gather`      — re-probe host info
* `POST /api/v1/sessions/dispatch`        — fan a command to every `group_dispatch=true` session

## Files (on the managed host)

* `GET  /api/v1/sessions/:id/files?path=` — read a file (chunked via offset+size)
* `POST /api/v1/sessions/:id/files?path=` — write / append bytes
* `GET  /api/v1/sessions/:id/files/size?path=` — stat size

## Tunnels

* `GET  /api/v1/sessions/:id/tunnels` — list active tunnels for one agent
* `POST /api/v1/sessions/:id/tunnels` — `{mode, src_address, dst_address}`
  where `mode ∈ {pull, push, dynamic, internet}`

## WebSocket

* `GET /notify?ticket=…`  — server-push events (new session, duplicate, …)
* `GET /ws/:hash?ticket=…` — interactive terminal for an agent
