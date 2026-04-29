# Frontend IA Redesign — Vercel Top-Bar + VSCode-style HostView

## Status (2026-04)

- ✅ **C1** — replace left sidebar with Vercel-style top bar
  (TopBar + NavTabs); deleted ProjectSidebar / TopChrome / CmdKHint.
- ✅ **C2** — split Audit into History (Activities, Recordings) +
  Operations (Transfers, Enrollment); deleted AuditPage; added
  `/audit/*` redirects.
- ✅ **C3a** — fix top-bar breadcrumb order to outer→inner (server
  before project).
- ⏳ **C3b** — promote `ManageServersDialog` to `/servers` page; add
  Admin global top-tab; group `/admin/*` under an `AdminLayout` with
  Users / Access Control / Settings sub-tabs.
- ⏳ **C4** — `FleetPage` parent route + sub-tabs (Hosts / Sessions /
  Topology / Approvals) + master-detail `HostsView`.
- ⏳ **C5** — VSCode-style HostView (ActivityBar + ActivityPane +
  collapsible BottomPanel).

The mockups + file-level changes for the remaining iterations are in
§14 (route tree), §15 (files to modify), §16 (components to reuse),
§17 (verification) below.

---

## Context

Today's chrome problems:

1. **One overloaded shell** mixes global pages (`/admin/*`, `/account`,
   `/preferences`) with project pages, blurring scope. Admin has no
   left-rail entry — only reachable via UserMenu.
2. **Audit hub is a misnomer** — it bundles read-only history (Activities,
   Recordings) with write-capable surfaces (Transfers, Enrollment).
3. **Fleet is one page with 4 mounted views, plus a sibling top-level
   `HostView`**. Switching hosts loses Fleet context. The view toggle
   conflates "list shape" with "different lens".
4. **A previous draft of this plan** put a left rail in *both* a Global
   and a Project shell — Brand / ServerSwitcher / UserMenu got duplicated
   and the IA felt heavier than it needed to be.

This plan adopts:

- **Vercel-style flat top nav** — a single thin top bar holds shell
  identity (Brand + Switchers + UserMenu) and a horizontal primary nav.
  No left sidebar at the shell level. Same chrome for both global and
  project contexts; only the switcher contents and the nav-row tabs
  change.
- **Audit split** into **History** (read-only) + **Operations** (write).
- **Fleet master-detail** with the host detail rendered in a **lightweight
  VSCode layout** (Activity Bar + active-activity content + collapsible
  bottom Terminal panel).

The rest of this doc is **visual mockups first**, structural details after.
Symbols: `●` online, `◯` offline, `▾` dropdown, `▶` selected row,
`⟳` refresh, `🔍` search. Diagrams target a 1280-wide window.

---

## 1. Top bar — the only persistent chrome

The same chrome renders everywhere; only the switcher row contents and
the nav-row tabs change with context.

### Project context (`/projects/acme-prod/...`)

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ◇ Platypus  /  acme-prod ▾  /  srv-prod-01.lan ▾                       ⌘K    ◐ alice ▾  │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Overview · Fleet · Operations · History · Members · Settings                              │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

### Global context (no project)

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ◇ Platypus  /  srv-prod-01.lan ▾                                       ⌘K    ◐ alice ▾  │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Projects · Servers · Admin                                                                │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

What lives where:
- **Brand**: `◇ Platypus`, always clickable, links to `/projects`.
- **Breadcrumb-style switchers**: `acme-prod ▾` opens ProjectSwitcher
  (only visible in project context); `srv-prod-01.lan ▾` opens
  ServerSwitcher (always visible).
- **Right cluster**: `⌘K` opens CommandPalette; `◐ alice ▾` opens UserMenu
  (Account, Preferences, Logout). Admin links *do not* live in the user
  menu anymore — they're a top-level nav-row tab in global context.
- **Nav row** (project): Overview · Fleet · Operations · History ·
  Members · Settings.
- **Nav row** (global): Projects · Servers · Admin. The Admin tab has its
  own sub-tabs (Users · Access Control · Settings) when active.

This eliminates the duplicated Brand/Switcher/UserMenu of the previous
two-sidebar draft.

---

## 2. Global pages — Projects landing & Admin

### Projects landing (`/projects`)

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ◇ Platypus  /  srv-prod-01.lan ▾                                       ⌘K    ◐ alice ▾  │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Projects · Servers · Admin                                                                │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Projects                                                                  [+ New project]│
│  🔍 search projects                                                              4 total  │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  ┌────────────────────────┐  ┌────────────────────────┐  ┌────────────────────────┐      │
│  │ acme-prod              │  │ acme-staging           │  │ research-lab           │      │
│  │ 12 hosts · 8 online    │  │ 4 hosts · 4 online     │  │ 28 hosts · 19 online   │      │
│  │ Updated 12m ago        │  │ Updated 3h ago         │  │ Updated 4d ago         │      │
│  └────────────────────────┘  └────────────────────────┘  └────────────────────────┘      │
│  ┌────────────────────────┐                                                              │
│  │ infra-edge             │                                                              │
│  │ 0 hosts                │                                                              │
│  │ never                  │                                                              │
│  └────────────────────────┘                                                              │
└──────────────────────────────────────────────────────────────────────────────────────────┘
[ status bar: 4 projects · 0 sessions · 14:23 UTC ]
```

### Admin (`/admin/users`) — Admin nav-tab opens its own sub-tab strip

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ◇ Platypus  /  srv-prod-01.lan ▾                                       ⌘K    ◐ alice ▾  │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Projects · Servers · Admin                                                                │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Admin                                                                  [+ Invite user ]  │
│  Users · Access Control · Settings                                                         │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  🔍 search        7 users · 2 admins · 1 disabled                                      ⟳ │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   username       role        2FA      last seen        actions                            │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   alice          admin       on       now              [ ⋯ ]                              │
│   bob            operator    on       4m ago           [ ⋯ ]                              │
│   carol          operator    off      2h ago           [ ⋯ ]                              │
│   dave           viewer      on       1d ago           [ ⋯ ]                              │
│   eve            operator    on       3d ago           [ ⋯ ]                              │
│   frank          admin       on       5d ago           [ ⋯ ]                              │
│   ghost          viewer      —        disabled         [ ⋯ ]                              │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

`/admin/access-control` and `/admin/settings` use the same chrome with
their own bodies. `/account` and `/preferences` reuse the global shell
but are reached only from UserMenu (no top tab) — they're personal
settings, not navigational destinations.

---

## 3. Project Overview (`/projects/acme-prod/overview`)

ProjectSwitcher appears in the breadcrumb; the nav row shows project
tabs. No left sidebar.

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ◇ Platypus  /  acme-prod ▾  /  srv-prod-01.lan ▾                       ⌘K    ◐ alice ▾  │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Overview · Fleet · Operations · History · Members · Settings                              │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Overview · acme-prod                                                                      │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐                                           │
│  │ Hosts      │  │ Online     │  │ Live shells│                                           │
│  │ 12         │  │ 8          │  │ 5          │                                           │
│  │ +1 today   │  │ 67 %       │  │ 2 idle     │                                           │
│  └────────────┘  └────────────┘  └────────────┘                                           │
│                                                                                            │
│  ┌─────────────────────────────────────┐  ┌────────────────────────────────────────────┐  │
│  │ Sessions · last 24 h                │  │ Top hosts (sessions)                       │  │
│  │  ▁▂▃▅▇█▆▄▂▃▅▇▅▃▂▁▂▃▅▇▆▄▂▁         │  │ prod-edge-01   ████████                    │  │
│  │  00:00              12:00     now   │  │ prod-edge-02   ██████                      │  │
│  └─────────────────────────────────────┘  │ prod-db-01     ████                        │  │
│                                            │ staging-app-1  ███                         │  │
│  ┌─────────────────────────────────────┐  │ research-gpu   ██                          │  │
│  │ Recent activity                     │  └────────────────────────────────────────────┘  │
│  │ 14:23  alice opened shell on …      │                                                  │
│  │ 14:21  bob uploaded deploy.sh …     │                                                  │
│  │ 14:18  prod-edge-02 came online     │                                                  │
│  └─────────────────────────────────────┘                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────┘
[ 12 hosts · 19 sessions · 2 transfers · 14:23 UTC ]
```

The body is exactly today's `ProjectOverview.tsx` content — only the
shell around it changes.

---

## 4. Fleet — Hosts tab, no selection (`/fleet/hosts`)

`/fleet` redirects to `/fleet/hosts`. Inside Fleet, a third nav row
holds the four sub-tabs.

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ◇ Platypus  /  acme-prod ▾  /  srv-prod-01.lan ▾                       ⌘K    ◐ alice ▾  │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Overview · Fleet · Operations · History · Members · Settings                              │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Fleet                          ●8 online · ◯4 offline · ⚠2 pending     [+ Enroll agent ]│
│  Hosts · Sessions · Topology · Approvals (2)                                              │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  🔍 search                            12 hosts             [ Cards │ Table ]            ⟳│
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   ●  alias                  os         ip               last seen     sessions            │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   ●  prod-edge-01          ubuntu24    10.0.0.5         12s ago       2                  │
│   ●  prod-edge-02          ubuntu24    10.0.0.6          4s ago       1                  │
│   ●  prod-edge-03          ubuntu24    10.0.0.7          8s ago       0                  │
│   ◯  prod-db-01            debian12    10.0.1.5          2h ago       0                  │
│   ●  staging-app-01        rocky9      10.1.0.5         16s ago       0                  │
│   ●  staging-app-02        rocky9      10.1.0.6          3s ago       1                  │
│   ●  research-gpu-01       ubuntu22    10.2.0.5          9s ago       1                  │
│   ◯  research-gpu-02       ubuntu22    10.2.0.6          3d ago       0                  │
│   ●  edge-failover-1       alpine3.19  10.0.2.5         28s ago       0                  │
│   ●  edge-failover-2       alpine3.19  10.0.2.6         11s ago       0                  │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

The Cards / Table toggle is *inside* the Hosts tab (persisted via
`usePreference("fleet.hostsView")`). Cards view replaces the body with
the existing `HostsCardPanel` grid.

---

## 5. Fleet — Hosts master-detail with VSCode HostView ★

URL: `/fleet/hosts/prod-edge-02/files`. The list collapses to a 260px
rail; the right pane is the **VSCode-style HostView**: Activity Bar +
active-activity content + collapsible bottom Terminal panel.

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ◇ Platypus  /  acme-prod ▾  /  srv-prod-01.lan ▾                       ⌘K    ◐ alice ▾  │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Overview · Fleet · Operations · History · Members · Settings                              │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Fleet                          ●8 online · ◯4 offline · ⚠2 pending     [+ Enroll agent ]│
│  Hosts · Sessions · Topology · Approvals (2)                                              │
├──────────────────┬───────────────────────────────────────────────────────────────────────┤
│ 🔍 search        │ ←  prod-edge-02   ●online   ubuntu24   10.0.0.6        ⌘P  ⌘\  ⌃`    │
│ ──────────────── │ ─┬─────────────────────────────────────────────────────────────────── │
│ ● prod-edge-01   │  │ EXPLORER · /home/ubuntu                ▶ ▾ ⟳         deploy.sh  ×  │
│▶● prod-edge-02   │📁│ ▾ ubuntu                                    1  #!/bin/bash         │
│ ● prod-edge-03   │ⓘ│  ▸ .ssh                                     2  set -euo pipefail    │
│ ◯ prod-db-01     │⊟│  ▸ logs                                     3                       │
│ ● staging-app-01 │⊞│  ▸ .cache                                   4  ./deploy --tgt prod  │
│ ● staging-app-02 │🌐│  ▤ deploy.sh        2.1K  rwx              5                       │
│ ● research-gpu-01│  │  ▤ stack.yml         824B  rw                                       │
│ ◯ research-gpu-02│  │  ▤ .bashrc          3.7K  rw                                       │
│ ◯ research-gpu-03│  │  ▤ id_rsa.pub        571B  rw           ────────────────────────── │
│ ● edge-failover-1│  │                                          PANEL: Terminal ▾        × │
│ ● edge-failover-2│  │                                          alice@prod-edge-02:~$ ls   │
│ ◯ edge-failover-3│  │                                          deploy.sh  stack.yml      │
│                  │  │                                          alice@prod-edge-02:~$ _   │
└──────────────────┴──┴────────────────────────────────────────────────────────────────────┘
[ 12 hosts · 19 sessions · 2 transfers · 14:23 UTC ]                              ⌘ K
```

Anatomy of the right pane:

```
HostView (right of master rail)
┌─ Host header bar ─────────────────────────────────────────────────────┐
│  ←  prod-edge-02   ●online   ubuntu24   10.0.0.6      ⌘P  ⌘\  ⌃`     │
└───────────────────────────────────────────────────────────────────────┘
┌─Activity─┬─ Active activity content (varies) ───────────────────────────┐
│   📁     │  EXPLORER (file tree)  │  Editor / Viewer (file body)         │
│   ⓘ      │  ─────────────────────  │  (file tabs only when ≥1 open)       │
│   ⊟      │                         │                                       │
│   ⊞      │                         │                                       │
│   🌐     │                         │                                       │
└──────────┴─────────────────────────┴──────────────────────────────────────┘
┌─ Bottom Panel (collapsible) ──────────────────────────────────────────┐
│  Terminal ▾    Processes    Tunnels                                  × │
│  alice@prod-edge-02:~$ _                                               │
└───────────────────────────────────────────────────────────────────────┘
```

- **Activity Bar** (44px, vertical icons): Files, Info, Sessions,
  Processes, Tunnels. Click switches the active activity; URL goes
  `/fleet/hosts/<id>/<activity>`. The icons keep the same registry
  (`lib/icons.ts`).
- **Active activity content** fills the rest. Files = tree+viewer split
  (the existing `FilesTab` already renders this); Info / Sessions =
  single column content. Lightweight: **no multi-pane editor groups**.
- **Bottom Panel**: collapsible (toggle: `⌃\``). Tabs inside: Terminal
  (always available — replaces the global TerminalDrawer when scoped to
  one host), Processes (lifted out of activity bar so it can be glanced
  while editing files), Tunnels (same). Default collapsed; auto-expands
  when an action requires it (open shell, start tunnel).
- **Host header bar**: `← Hosts` returns to `/fleet/hosts`; identity
  pills show online/os/ip; right-side keybinds reflect the VSCode hot
  keys (Cmd+P / Cmd+\ split / Ctrl+\` toggle panel — all opt-in, no
  hard wiring required for v1).

Master rail behavior:
- Width 260 px when host selected; full width when no selection.
- Selected row marked with `▶` and `scrollIntoView({block:'nearest'})`.
- Search and scroll persist when switching between hosts.
- Hidden under 960 px viewport; replaced by `← Hosts` back button.

---

## 6. HostView activities — what each one looks like

### 6a. Files (default activity)

```
┌─Act─┬─ EXPLORER ────────────┬─ deploy.sh ×   stack.yml   ─────────────────────┐
│ 📁● │ ▾ /home/ubuntu        │   1  #!/bin/bash                                 │
│ ⓘ   │  ▸ .ssh               │   2  set -euo pipefail                           │
│ ⊟   │  ▸ logs               │   3                                              │
│ ⊞   │  ▤ deploy.sh          │   4  ./deploy --target prod                      │
│ 🌐  │  ▤ stack.yml          │   5                                              │
│     │  ▤ .bashrc            │                                                  │
└─────┴───────────────────────┴──────────────────────────────────────────────────┘
```

(File tree is the only "side bar" content for Files. The viewer/editor
opens in the right column; the small file-tabs strip lets users swap
between recently opened files. Minimal: ≤6 open at once, no split editor
groups.)

### 6b. Info

```
┌─Act─┬─ Host overview ───────────────────────────────────────────────────────┐
│ 📁  │  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐               │
│ ⓘ ● │  │ os            │  │ kernel        │  │ uptime        │               │
│ ⊟   │  │ ubuntu 24.04  │  │ 6.18.5        │  │ 18 days       │               │
│ ⊞   │  └───────────────┘  └───────────────┘  └───────────────┘               │
│ 🌐  │  ┌─────────────────────────────────────────────────────────────────┐    │
│     │  │ CPU · 8 cores · 23 % avg                                        │    │
│     │  │ MEM · 16 GiB total · 4.2 GiB used                               │    │
│     │  │ DISK · /  240 GiB · 38 % used                                   │    │
│     │  │ NET  · eth0  10.0.0.6  ↑ 1.2 MB/s  ↓ 3.4 MB/s                   │    │
│     │  └─────────────────────────────────────────────────────────────────┘    │
│     │  [ Upgrade agent ]   [ Restart agent ]   [ Disconnect ]                  │
└─────┴───────────────────────────────────────────────────────────────────────┘
```

(Existing `InfoTab` content; lays into the activity area as a single
column.)

### 6c. Sessions (per-host)

```
┌─Act─┬─ Sessions on prod-edge-02 ─────────────────────────────────────────┐
│ 📁  │  ●live   alice    /bin/bash    since 14:18    [ open ]             │
│ ⓘ   │  ●live   bob      /bin/zsh     since 14:09    [ open ]             │
│ ⊟ ● │  ─────────── earlier ────────────                                  │
│ ⊞   │  ◯end    alice    /bin/bash    13:55 → 14:00                       │
│ 🌐  │  ◯end    bob      /bin/zsh     11:14 → 11:48                       │
└─────┴──────────────────────────────────────────────────────────────────┘
```

### 6d. Processes  &  6e. Tunnels

Both render their existing tab bodies (`ProcessesTab`, `TunnelsTab`)
into the activity content area. Worth noting: even when `Processes` is
the active activity in the side bar, the bottom Panel can also surface
a `Processes` tab — they share the same data, just different framings
(Activity = focused full-pane; Panel = peek-while-editing).

---

## 7. Fleet — Sessions tab (`/fleet/sessions`)

Cross-host live + recent sessions.

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ … top bar + project tabs …                                                                │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Fleet                                                                  [+ Enroll agent ]│
│  Hosts · Sessions · Topology · Approvals (2)                                              │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  🔍 search                              Live · Last 24h · Last 7d                       ⟳│
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   ●live   alice@prod-edge-02         /bin/bash         since 14:18                        │
│   ●live   bob@prod-edge-01           /bin/zsh          since 14:09                        │
│   ●live   alice@staging-app-02       /bin/bash         since 13:55                        │
│   ●live   carol@research-gpu-01      /bin/bash         since 13:41                        │
│   ●live   bob@prod-edge-03           /bin/sh           since 13:20                        │
│  ────────────────────── earlier ──────────────────────                                    │
│   ◯end    alice@prod-db-01           /bin/bash         12:31 → 13:07                      │
│   ◯end    eve@staging-app-01         /bin/bash         11:14 → 11:48                      │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

Click a row: live session → opens GlobalTerminal drawer; ended → opens
`/history/recordings/<id>`.

---

## 8. Fleet — Topology tab (`/fleet/topology`)

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│  Fleet                                                                  [+ Enroll agent ]│
│  Hosts · Sessions · Topology · Approvals (2)                                              │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   Layout: ◉force ◯hierarchical    Filter: all      3.2 KB/s              ⟳              │
│  ─────────────────────────────────────────────────┬──────────────────────────────────── │
│                                                   │ Selected: prod-edge-02              │
│              ┌─────┐                              │ ─────────────────────────────────── │
│              │ srv │                              │ os    ubuntu24                      │
│              └──┬──┘                              │ ip    10.0.0.6                      │
│         ┌──────┼──────┐                           │ since 14:02                         │
│         │      │      │                           │ peers 3                             │
│      ┌──▼─┐ ┌──▼─┐ ┌──▼─┐                         │ ─────────────────────────────────── │
│      │p01 │ │p02 │ │p03 │                         │ Open shell · Files · Info           │
│      └────┘ └─┬──┘ └────┘                         │                                     │
│              │                                    │                                     │
│           ┌──▼──┐                                 │                                     │
│           │ db  │                                 │                                     │
│           └─────┘                                 │                                     │
└───────────────────────────────────────────────────┴──────────────────────────────────────┘
```

Quick actions in the right detail panel deep-link to
`/fleet/hosts/<id>/<activity>` — they re-enter the master-detail surface.

---

## 9. Fleet — Approvals tab (`/fleet/approvals`)

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│  Fleet                                                                  [+ Enroll agent ]│
│  Hosts · Sessions · Topology · Approvals (2)                                              │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  2 pending · auto-expire 24 h                                                          ⟳ │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   alias            requested      ip          token             actions                   │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   new-edge-04      2m ago         10.0.0.8    ed25519:7fe9      [✓ Approve]  [✗ Reject]   │
│   stg-app-03       18m ago        10.1.0.7    ed25519:9a2b      [✓ Approve]  [✗ Reject]   │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 10. History (`/history`) — read-only audit

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│  Overview · Fleet · Operations · History · Members · Settings                              │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  History                                                                                   │
│  Activities · Recordings                                                                   │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  🔍 search          actor·any   type·any   range·24h                                    ⟳│
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   when     actor      action                          target                              │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   14:23    alice      shell.open                      prod-edge-02                        │
│   14:21    bob        file.upload  deploy.sh          prod-edge-02                        │
│   14:18    —          host.online                     prod-edge-02                        │
│   14:15    carol      approval.grant                  staging-app-03                      │
│   14:09    bob        shell.open                      prod-edge-01                        │
│   13:55    alice      shell.open                      staging-app-02                      │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

`Recordings` sub-tab is the same chrome with the existing
`RecordingsPage` body.

---

## 11. Operations (`/operations`) — write-capable runtime state

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│  Overview · Fleet · Operations · History · Members · Settings                              │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│  Operations                                                                                │
│  Transfers · Enrollment                                                                    │
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│  Active 3 · queued 1 · throughput 1.4 MB/s                                              ⟳│
│  ──────────────────────────────────────────────────────────────────────────────────────  │
│   ↑ deploy.sh           prod-edge-02       ████████░░    82 %      21 KB/s                │
│   ↓ /var/log/syslog     prod-edge-02       ███░░░░░░░    31 %      12 KB/s                │
│   ↑ stack.yml           staging-app-1      ██████████   100 %       done                  │
│   ⏸ artifacts.tgz       prod-db-01         queued                                          │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

`Enrollment` sub-tab: token + artifact management body (existing
`EnrollmentPage`).

```
   Transfers · Enrollment
  ─────────────────────────────────────────────────────────────────────────
   3 active tokens · 7 used · 12 expired                                  ⟳
  ─────────────────────────────────────────────────────────────────────────
   token              issued by   scope         used     expires        act
   ed25519:7fe9...    alice       acme-prod     0/1      in 23 h     [revoke]
   ed25519:9a2b...    bob         acme-prod     0/1      in 11 h     [revoke]
   ed25519:1c4d...    alice       acme-prod     1/1      used 2h ago   [-]
```

---

## 12. Narrow viewport (< 960 px) — Fleet master-detail collapse

```
viewport ≥ 960 px (split)              viewport < 960 px (selection hides rail)
┌───────────┬──────────────────┐        ┌──────────────────────────────────────┐
│ rail (260)│ HostView          │        │ ←  prod-edge-02  ●  ubuntu24         │
│ ● host α  │ Activity + content│        │ ──────────────────────────────────── │
│▶● host β  │                   │        │ ┌──┐                                  │
│ ● host γ  │                   │        │ │📁│  EXPLORER                        │
│ ◯ host δ  │                   │        │ │ⓘ│                                   │
└───────────┴──────────────────┘        │ │⊟│                                   │
                                        │ │⊞│                                   │
                                        │ │🌐│                                   │
                                        │ └──┘                                  │
                                        └──────────────────────────────────────┘
```

`← Hosts` button = `navigate("/projects/<slug>/fleet/hosts")` (returns
to list-only). Implemented with `useMediaQuery("(max-width: 960px)")`.

---

## 13. Before vs after — chrome silhouette

```
BEFORE (left-rail shell, today)              AFTER (top-bar shell)
┌─────────┬──────────────────────────┐      ┌──────────────────────────────────────────┐
│ ◇       │  PageHeader              │      │ ◇  acme-prod ▾  srv-prod ▾    ⌘K  ◐ alice│
│ proj ▾  │                          │      │ Overview · Fleet · Ops · History · Mem · │
│ srv  ▾  │                          │      ├──────────────────────────────────────────┤
│         │                          │      │ <page>                                   │
│ Work    │                          │      │                                          │
│ ▣ Over  │                          │      │                                          │
│ ⊟ Fleet │                          │      │                                          │
│         │                          │      │                                          │
│ Admin   │                          │      │                                          │
│ ▤ Memb  │                          │      │                                          │
│         │                          │      │                                          │
│ Audit   │                          │      │                                          │
│ ⏱ Audit │                          │      │                                          │
│         │                          │      │                                          │
│ Project │                          │      │                                          │
│ ⚙ Set   │                          │      │                                          │
│ ───     │                          │      │                                          │
│ ◐ user  │                          │      │                                          │
└─────────┴──────────────────────────┘      └──────────────────────────────────────────┘
~220 px sidebar always present              0 px lateral chrome — full width for content
```

Vertical real estate spent on chrome: today ~64 px (header) + 0 lateral
side-bar pixels are content width-eating. After the change: 48 px top
bar + 40 px nav row = 88 px vertical. Trade ~24 vertical pixels for
220 lateral pixels — a clear win on widescreen, neutral on narrow.

---

## 14. Route tree (target)

```
/login
/onboarding

(GlobalShell  =  TopBar with global nav row)
  /projects                              ProjectsLanding
  /servers                               Servers (promoted from dialog)
  /admin                                 → admin/users
    users                                AdminUsers
    access-control                       AdminAccessControl
    settings                             AdminSettings
  /account                               Account     (UserMenu only)
  /preferences                           Preferences (UserMenu only)

(ProjectShell = TopBar with project nav row, requireProject)
  /projects/:slug                        → overview
    overview                             ProjectOverview
    fleet                                FleetPage (Outlet)
      → hosts
      hosts                              HostsView (master-detail outlet)
        → hosts/:hostId/files
        :hostId                          → :hostId/files
        :hostId/:activity                HostView (VSCode-style)
                                         (activity = files|info|sessions|
                                          processes|tunnels)
      sessions                           SessionsPanel
      topology                           TopologyPanel
      approvals                          ApprovalsPage
    operations                           → operations/transfers
      transfers                          TransfersPage
      enrollment                         EnrollmentPage
    history                              → history/activities
      activities                         ActivitiesPage
      recordings                         RecordingsPage
    members                              ProjectMembers
    settings                             ProjectSettings

Backwards-compat redirects (in routes.tsx):
  /projects/:slug/hosts/:id              → /projects/:slug/fleet/hosts/:id/files
  /projects/:slug/hosts/:id/:tab         → /projects/:slug/fleet/hosts/:id/:tab
  /projects/:slug/audit                  → /projects/:slug/history
  /projects/:slug/audit/activities       → /projects/:slug/history/activities
  /projects/:slug/audit/recordings       → /projects/:slug/history/recordings
  /projects/:slug/audit/transfers        → /projects/:slug/operations/transfers
  /projects/:slug/audit/enrollment       → /projects/:slug/operations/enrollment
  /projects/:slug/fleet/enroll           → /projects/:slug/operations/enrollment
  /projects/:slug/enrollment             → /projects/:slug/operations/enrollment
```

Note: tab name "tab" → "activity" inside HostView is a label change in
docs / code comments; the URL slugs (files / info / sessions /
processes / tunnels) stay the same so deep links keep working.

---

## 15. Files to modify

### Chrome (drop the sidebars; introduce a single top bar)

- **`layout/AppShell.tsx`** (new, replaces `ProjectShell` + the would-be
  `GlobalShell`) — single shell that renders `TopBar`, `<Outlet />`,
  and the existing `StatusBar` + `TerminalDrawer` + `CommandPalette` +
  `TransfersDrawer`. Holds the project list context the way
  `ProjectShell` does today.
- **`layout/TopBar.tsx`** (new) — Brand · ProjectSwitcher (when in
  project) · ServerSwitcher · spacer · CmdK trigger · UserMenu.
- **`layout/NavTabs.tsx`** (new) — horizontal tab strip; renders the
  project tabs or global tabs based on URL. Uses `components/ui/tabs`.
- **Delete `layout/ProjectShell.tsx`**, **`layout/ProjectSidebar.tsx`**,
  **`layout/TopChrome.tsx`** — replaced by AppShell + TopBar + NavTabs.
- **`layout/UserMenu.tsx`** — drop Admin links; keep Account /
  Preferences / Logout. Single instance, mounted inside TopBar.
- **`layout/ProjectSwitcher.tsx` / `ServerSwitcher.tsx`** — visual
  refit for inline breadcrumb form; keep current popover + actions.

### Routes
- **`routes.tsx`** — restructure into the tree from §14; add redirects.
- **`routes/HostViewRoute.tsx`** — same body, new path.

### Fleet
- **`pages/FleetPage.tsx`** — convert to parent route: PageHeader +
  sub-tab strip (Hosts / Sessions / Topology / Approvals) +
  `<Outlet />`. Mount `EnrollAgentWizard` and `EnrollmentWaitBanner`
  here. Drop the old display:none-stack of 4 mounted views.
- **`pages/fleet/HostsView.tsx`** (new) — master-detail. Reuses
  `HostsPanel` (rail form) on the left, `<Outlet />` on the right.
- **`pages/fleet/HostsPanel.tsx`** — add `compact` prop for the rail
  form (no IP / OS columns; just status + alias + last seen).
- **`pages/fleet/SessionsPanel.tsx`**, **`TopologyPanel.tsx`** — already
  full panels; just hook up at the new routes.

### HostView (VSCode-style)
- **`pages/HostView.tsx`** — restructure: split into `HostHeaderBar`
  (back link + identity), `ActivityBar` (vertical icons), `ActivityPane`
  (renders the active tab body), and `BottomPanel` (Terminal /
  Processes / Tunnels collapsible).
- **`pages/host/ActivityBar.tsx`** (new) — 44 px vertical icon strip,
  uses `lib/icons.ts`.
- **`pages/host/BottomPanel.tsx`** (new) — collapsible panel with
  internal tabs; integrates the existing `GlobalTerminalContext` so
  shells already attached stay alive on activity switch.
- **`pages/host/FilesTab.tsx`** — already does tree+viewer split;
  keep. Add a small "open files" tabs strip at the top of the viewer
  (≤6, no split editor groups).
- **`pages/host/InfoTab.tsx`**, **`SessionsTab.tsx`**, **`ProcessesTab.tsx`**,
  **`TunnelsTab.tsx`** — bodies unchanged, just rendered inside
  `ActivityPane`.

### Audit → History / Operations
- **`pages/HistoryPage.tsx`** (new) — copy `AuditPage`'s tab-strip
  pattern; tabs: Activities, Recordings.
- **`pages/OperationsPage.tsx`** (new) — same pattern; tabs: Transfers,
  Enrollment.
- **Delete `pages/AuditPage.tsx`** once redirects ship.

### Servers
- **`pages/Servers.tsx`** (new) — promotes `ManageServersDialog` to a
  page; reuses the existing form components / actions (still callable
  as a dialog from `ServerSwitcher`).

### Icons
- **`lib/icons.ts`** — add `history`, `operations`, `activity-files`,
  `activity-info`, `activity-sessions`, `activity-processes`,
  `activity-tunnels` keys. Mark `audit` deprecated in a comment.

---

## 16. Reused components — do not duplicate

| Need                               | Reuse                                                    |
|------------------------------------|----------------------------------------------------------|
| Page chrome / header               | `components/PageShell.tsx`, `PageHeader.tsx`             |
| Sub-tab strips                     | `components/ui/tabs` (already used by AuditPage)         |
| Search + facets + count + ⟳        | `components/FilterToolbar.tsx`                           |
| KPI tiles                          | `components/MetricCard.tsx`                              |
| Charts                             | `components/charts/LineChartCard.tsx`, `BarChartCard.tsx`|
| Mesh graph                         | `components/topology/MeshGraph.tsx`                      |
| Status badges in header            | `components/StatusPills.tsx`                             |
| Per-user toggle (cards/table)      | `lib/preferences.ts` `usePreference`                     |
| Cmd-K                              | `layout/CommandPalette.tsx` (register new routes)        |
| Terminal session lifecycle         | `terminal/GlobalTerminalContext.ts` — wire into the new  |
|                                    | `BottomPanel` (replaces the floating drawer when a host  |
|                                    | is in scope; the global drawer remains for cross-host)   |

---

## 17. Verification

1. **Type + unit**: `cd desktop/frontend && pnpm test` and
   `pnpm tsc --noEmit`. Update fixtures listed below.
2. **E2E**: `make e2e`. Smoke specs:
   - `e2e/specs/56-sidebar-nav-grouping.spec.ts` — pins today's group
     order; rewrite for top-bar nav.
   - any spec hitting `/fleet`, `/audit/...`, `/hosts/...` (must hit
     redirects).
3. **Manual smoke (Wails dev)** — `cd desktop && wails dev`, walk:
   - Sign in → top-bar GlobalShell at `/projects` with tabs
     Projects · Servers · Admin (Admin only when admin+).
   - UserMenu (top right): Account / Preferences / Logout. No Admin
     links inside.
   - Open project → top bar shows `acme-prod ▾` switcher and the
     project nav row.
   - `/fleet` → redirects to `/fleet/hosts`; full-width Hosts list.
   - Click a host → URL `…/fleet/hosts/<id>/files`; rail (260 px) on
     left, VSCode-style HostView on right with Activity Bar +
     EXPLORER + viewer.
   - Click Activity Bar icons: switches activity (URL updates).
   - Toggle Bottom Panel (`⌃\`` or click the panel header): Terminal /
     Processes / Tunnels appear; Terminal hooks into existing
     GlobalTerminal session if one exists.
   - Switch between hosts via the rail: rail scroll + search persist.
   - Switch project nav rows: Operations · History · Members ·
     Settings all render.
   - Old URLs redirect: `/projects/<slug>/audit/recordings` →
     `/history/recordings`; `/projects/<slug>/hosts/<id>/info` →
     `/fleet/hosts/<id>/info`; `/projects/<slug>/fleet/enroll` →
     `/operations/enrollment`.
4. **Narrow viewport** (< 960 px): host detail collapses, master rail
   hides, `← Hosts` back button restores list.

### Tests to update
- `pages/FleetPage.test.tsx`
- `pages/Account.test.tsx`
- `pages/ProjectMembers.test.tsx`
- `pages/ProjectSettings.test.tsx`
- `pages/Login.test.tsx`
- `pages/Preferences.test.tsx`
- `routes/enrollmentRoute.test.tsx`
- `e2e/specs/56-sidebar-nav-grouping.spec.ts` (rename to
  `56-topbar-nav-grouping.spec.ts`)
