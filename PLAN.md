# Enrollment Wizard Refactor — Plan

## Overview

Two related IA changes to the desktop frontend:

1. **Inline enroll-agent wizard in Fleet card view.** A multi-step modal
   walks the user through OS → arch → server endpoint / advanced
   options → generated install command. The entry point is a tile
   embedded directly in the Fleet card grid (the empty space next to
   the host cards in the screenshot), plus the existing "Enroll agent"
   button in the Fleet header.

2. **Move enrollment management page under Audit.** The list of
   historical install artifacts and enrollment tokens (rarely visited
   day-to-day) moves from `/projects/<slug>/fleet/enroll` to
   `/projects/<slug>/audit/enrollment`. Old URLs redirect for
   back-compat.

## Goals & constraints

- The wizard is the primary "create install command" path. The existing
  `IssueInstallDialog` on the management page can either reuse the
  wizard or stay as a power-user form — keep it as-is for now to
  contain blast radius.
- Old URLs (`/fleet/enroll`, `/enrollment`) keep working as redirects
  so e2e specs that hit those paths don't need to be edited.
- Use a `?enroll=1` URL search param to drive open/close, so:
  - Header "Enroll agent" button is just a `<Link>` that sets the param
  - Inline tile is the same `<Link>`
  - Dismissing the wizard removes the param (no extra state plumbing)
  - Deep links land directly on the wizard
- Sidebar shape stays the same (Work · Admin · Audit · Project). The
  existing `audit/activities` link continues to drive the Audit group;
  the new `audit/enrollment` is reached via the Audit page's tab strip.

## New components

### `desktop/frontend/src/pages/fleet/EnrollAgentWizard.tsx`

Self-contained multi-step modal. Reads/writes `?enroll=1` to open/close.

Steps:

1. **Target OS** — toggle group of OSes from `listInstallPlatforms()`,
   filtered by what the active channel has published. Skipping this
   step is allowed (auto-detect at runtime).
2. **Target architecture** — toggle group of archs valid for the
   selected OS. Same skip-allowed semantics.
3. **Connection** — `server_endpoint` (prefilled from `getServerInfo`),
   TTL slider/input, auto-approve toggle, optional description.
4. **Run it** — renders `install_command` (curl|sh) with a tab to the
   offline `bundle_url` shape. Copy button + "I'll run this — show me
   Fleet" button that closes the wizard and switches to
   `?await=enroll`.

The four-step layout reuses the existing `Dialog` primitive plus a
custom step indicator strip at the top. Back / Next buttons in the
footer; Next is disabled until the current step is valid (or skipped
deliberately).

### Inline tile in `HostsCardPanel.tsx`

Renders as the **first** card in the grid so an empty fleet shows just
the wizard tile (no special empty-state branch needed). Visually
matches the host-card padding / radius / typography, but uses a dashed
border and centered "+ Enroll agent" label to read as an action card.
Clicks navigate to `?enroll=1` so the wizard opens on top of the grid.

## Routing changes (`src/routes.tsx`)

Before:

```
fleet/enroll    → EnrollmentPage         (canonical)
enrollment      → Navigate to fleet/enroll
audit/          → AuditPage
  ├ activities  → ActivitiesPage
  ├ recordings  → RecordingsPage
  └ transfers   → TransfersPage
```

After:

```
audit/          → AuditPage
  ├ activities  → ActivitiesPage
  ├ recordings  → RecordingsPage
  ├ transfers   → TransfersPage
  └ enrollment  → EnrollmentPage         (canonical, NEW)
fleet/enroll    → Navigate to ../audit/enrollment
enrollment      → Navigate to audit/enrollment
```

`AuditPage.tsx` adds `"enrollment"` to its `TABS` tuple with label
`"Enrollment"`.

## Wizard wiring

`FleetPage.tsx` mounts `<EnrollAgentWizard />` once at the page level
(so it floats above whichever child view is active — cards / table /
timeline / graph). The header "Enroll agent" button becomes a Link to
`?enroll=1`. The inline tile in `HostsCardPanel` does the same.

`CommandPalette.tsx` repoints "Enroll agent" from `fleet/enroll` to
`fleet?enroll=1` so the palette also opens the wizard rather than a
full-page route.

## Test changes

- `routes/enrollmentRoute.test.tsx` — assert canonical URL is now
  `audit/enrollment` and that both legacy paths redirect there.
- `pages/FleetPage.test.tsx` — "Enroll agent" link now points at
  `?enroll=1` (relative), not `/fleet/enroll`.
- `pages/EnrollmentPage.test.tsx` — unchanged copy assertions; still
  exercise the same component.
- `layout/ProjectSidebar.test.tsx` — unchanged.
- New `pages/fleet/EnrollAgentWizard.test.tsx` covers:
  - URL param drives open state
  - OS step → arch step → connect step → result step navigation
  - Skipping OS/arch results in `target_os` / `target_arch` empty in
    the request payload
  - Result step renders the returned `install_command`

E2E specs (`/projects/.../fleet/enroll`, `/projects/.../enrollment`)
keep working because both URLs now redirect to the same component
mounted under audit. No edits needed unless an assertion checks
`page.url()` mid-test.

## Implementation order

1. Add `EnrollAgentWizard.tsx` (wizard + URL-param hook).
2. Add inline tile to `HostsCardPanel.tsx`.
3. Mount wizard in `FleetPage.tsx`; rewire header button to
   `?enroll=1`.
4. Update `routes.tsx` redirects + canonical URL.
5. Update `AuditPage.tsx` tabs.
6. Update `CommandPalette.tsx`.
7. Adjust `routes/enrollmentRoute.test.tsx` and
   `pages/FleetPage.test.tsx` to the new shape; add wizard test.
8. Run `pnpm test` + `pnpm typecheck`; commit; push.
