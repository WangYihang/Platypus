# Platypus UI demos

Auto-generated from `e2e/specs/_demo/*.demo.ts`. Re-run with `pnpm run demos` from `e2e/`. Each clip is a Playwright spec played at slowMo=250 with in-page captions.

## First-run onboarding wizard

Fresh client → /onboarding → paste server URL → log in → land in /projects.

<video src="01-onboarding.webm" controls width="900"></video>

_807.4 KB · WebM (VP8/Opus)_

## Slack-style server rail

Add a second profile, switch with Ctrl+1 / Ctrl+2, rename via the themed Dialog (no native popups).

<video src="02-server-rail.webm" controls width="900"></video>

_1.4 MB · WebM (VP8/Opus)_

## Fleet · Table / Timeline / Graph

One Fleet route, three views toggled by URL. Hosts table → Sessions timeline → Mesh topology graph.

<video src="03-fleet-views.webm" controls width="900"></video>

_1.6 MB · WebM (VP8/Opus)_

## Global terminal drawer survives navigation

Open a shell on a host, walk through Activities and Settings, drawer keeps streaming — the bug that drove the refactor.

<video src="04-terminal-persistence.webm" controls width="900"></video>

_1.6 MB · WebM (VP8/Opus)_

## Cmd / Ctrl+K command palette

Keyboard-first nav: jump between pages, switch project, open a terminal on any host — all from the palette.

<video src="05-command-palette.webm" controls width="900"></video>

_1.7 MB · WebM (VP8/Opus)_

## Phase 1+2 auth redesign — opaque session + AAT lifecycle

End-to-end demonstration of the new auth path: log in via the UI to receive a single `pst_` session token (no JWT pair, no refresh dance), mint an `aat_` AI agent token via REST, use it to call a protected endpoint, revoke it, and watch the same bearer immediately fail — the verifier cache invalidates synchronously, so revocation is 0-latency on the issuing node.

<video src="06-auth-redesign.webm" controls width="900"></video>

_~743 KB · WebM (VP8/Opus)_
