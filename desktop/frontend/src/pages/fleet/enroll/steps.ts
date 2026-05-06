// Linear step model for the EnrollAgentWizard. Kept as a tiny
// const-tuple instead of an enum so step ordering is obvious at the
// callsite and TypeScript still catches typos.
//
// `PlatformsState` (the live install-manifest fetch lifecycle) lives
// in pages/enrollment/platforms.ts because the same shape is reused
// by the management-page picker; re-export through here for
// consumers inside the wizard subtree so they don't have to know
// where it physically lives.
export { type PlatformsState } from "../../enrollment/platforms";

export const STEPS = [
    "pick_preset",
    "server",
    "download_tls",
    "os",
    "arch",
    "ttl",
    "pat_max_uses",
    "auto_approve",
    "baseline_plugins",
    "description",
    "review",
    "run",
] as const;
export type Step = (typeof STEPS)[number];

export const STEP_LABEL: Record<Step, string> = {
    pick_preset: "Preset",
    server: "Server",
    download_tls: "TLS",
    os: "OS",
    arch: "Arch",
    ttl: "TTL",
    pat_max_uses: "PAT uses",
    auto_approve: "Approval",
    baseline_plugins: "Plugins",
    description: "Note",
    review: "Review",
    run: "Run",
};
