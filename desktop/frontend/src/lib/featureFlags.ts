// featureFlags — compile-time gates for in-progress UI.
//
// Features sit behind a flag while their backend lands on a staggered
// schedule. Flip to true once the full stack is wired and QA signs
// off. Kept dead-simple — no runtime toggle, no remote config — so
// there's no doubt about what a given build shipped.

export const featureFlags = {
    // Mesh + machine topology page. Backend (snapshot + 1 Hz stats +
    // time-series history) + frontend (Cytoscape + fcose + detail
    // panels) landed on branch claude/mesh-network-visualization-
    // OQDWB and are now enabled by default. The flag stays so the
    // sidebar entry can still be hidden in e.g. embedded tenants
    // that disable the mesh entirely.
    topology: true,
} as const;

export type FeatureName = keyof typeof featureFlags;
