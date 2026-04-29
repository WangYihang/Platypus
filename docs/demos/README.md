# Platypus UI demos

Auto-generated from `e2e/specs/_demo/*.demo.ts`. Re-run with `pnpm run demos` from `e2e/`. Each clip is a Playwright spec played at slowMo=250 with in-page captions.

## First-run onboarding wizard

Fresh client → /onboarding → paste server URL → log in → land in /projects.

<video src="01-onboarding.webm" controls width="900"></video>

_837.3 KB · WebM (VP8/Opus)_

## Slack-style server rail

Add a second profile, switch with Ctrl+1 / Ctrl+2, rename via the themed Dialog (no native popups).

<video src="02-server-rail.webm" controls width="900"></video>

_1.9 MB · WebM (VP8/Opus)_

## Fleet · Table / Timeline / Graph

One Fleet route, three views toggled by URL. Hosts table → Sessions timeline → Mesh topology graph.

<video src="03-fleet-views.webm" controls width="900"></video>

_1.6 MB · WebM (VP8/Opus)_

## Global terminal drawer survives navigation

Open a shell on a host, walk through Activities and Settings, drawer keeps streaming — the bug that drove the refactor.

<video src="04-terminal-persistence.webm" controls width="900"></video>

_2.1 MB · WebM (VP8/Opus)_

## Cmd / Ctrl+K command palette

Keyboard-first nav: jump between pages, switch project, open a terminal on any host — all from the palette.

<video src="05-command-palette.webm" controls width="900"></video>

_1.8 MB · WebM (VP8/Opus)_

## Phase-2 auth: opaque session-token lifecycle

Login → server returns a single `pst_…` bearer (no JWT pair). Logout revokes it server-side; subsequent requests fail with 401 immediately.

<video src="06-auth-redesign.webm" controls width="900"></video>

_600.0 KB · WebM (VP8/Opus)_
