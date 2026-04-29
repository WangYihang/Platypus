import { InstallPlatform } from "../../../lib/api";

// Linear step model for the EnrollAgentWizard. Kept as a tiny
// const-tuple instead of an enum so step ordering is obvious at the
// callsite and TypeScript still catches typos.
export const STEPS = ["os", "arch", "connect", "run"] as const;
export type Step = (typeof STEPS)[number];

export const STEP_LABEL: Record<Step, string> = {
    os: "OS",
    arch: "Arch",
    connect: "Connect",
    run: "Run",
};

// PlatformsState tracks the install-target picker's lifecycle so the
// UI can disable controls while loading and surface the right empty
// / error hint without conflating "no manifest published" with
// "request failed".
export type PlatformsState =
    | { status: "loading" }
    | { status: "ready"; platforms: InstallPlatform[]; channel: string }
    | { status: "empty"; channel: string }
    | { status: "error"; message: string };
