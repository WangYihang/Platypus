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

export const STEPS = ["os", "arch", "connect", "run"] as const;
export type Step = (typeof STEPS)[number];

export const STEP_LABEL: Record<Step, string> = {
    os: "OS",
    arch: "Arch",
    connect: "Connect",
    run: "Run",
};
