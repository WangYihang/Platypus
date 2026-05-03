// Capability family metadata. Mirrors the agent-side
// CapabilityID enum in internal/agent/plugin/manifest.go but adds
// human-facing labels + a coarse risk tier the UI uses to colour
// badges and order the checklist.
//
// Risk tiers — judged against "what an unrestricted misuse of this
// capability could let a hostile plugin do":
//   high   — exec arbitrary commands, write to disk, talk to network
//   medium — read potentially sensitive files (paths still scoped
//            by the manifest's `paths`)
//   low    — no host-side state mutation; per-plugin or read-only
//            of the agent's own metadata
//
// The agent's host fns enforce both the granted-set check and the
// path/command/host scope; this metadata is purely UX colouring.

export type CapabilityRisk = "low" | "medium" | "high";

export type CapabilityFamily =
    | "log"
    | "kv"
    | "sysinfo"
    | "fs.read"
    | "fs.write"
    | "exec"
    | "net.http";

export interface CapabilityMeta {
    family: CapabilityFamily;
    label: string;
    risk: CapabilityRisk;
    summary: string;
}

const CAPABILITY_META: Record<CapabilityFamily, CapabilityMeta> = {
    log: {
        family: "log",
        label: "Logging",
        risk: "low",
        summary: "Write audit lines to the agent's plugin log buffer.",
    },
    kv: {
        family: "kv",
        label: "Key/value storage",
        risk: "low",
        summary: "Persist small values in a per-plugin namespace on disk.",
    },
    sysinfo: {
        family: "sysinfo",
        label: "System info",
        risk: "low",
        summary: "Read host kernel / hostname / boot time. No file access.",
    },
    "fs.read": {
        family: "fs.read",
        label: "Filesystem read",
        risk: "medium",
        summary: "Read files within the paths declared in the manifest.",
    },
    "fs.write": {
        family: "fs.write",
        label: "Filesystem write",
        risk: "high",
        summary: "Create / modify / delete files within the declared paths.",
    },
    exec: {
        family: "exec",
        label: "Execute commands",
        risk: "high",
        summary: "Spawn the commands listed in the manifest.",
    },
    "net.http": {
        family: "net.http",
        label: "HTTP requests",
        risk: "high",
        summary: "Make outbound HTTP requests to the declared hosts.",
    },
};

export function capabilityMeta(family: string): CapabilityMeta {
    if (family in CAPABILITY_META) {
        return CAPABILITY_META[family as CapabilityFamily];
    }
    // Forward-compatible fallback: unknown family from a newer plugin
    // server. Treat as high-risk so an operator review is required and
    // keep the family string itself as the label so it's not mystery.
    return {
        family: family as CapabilityFamily,
        label: family,
        risk: "high",
        summary: "Capability family not known to this client. Treat as untrusted.",
    };
}

// Sort order: high-risk first so the operator's eyes land on the
// dangerous entries before they start clicking.
const RISK_ORDER: Record<CapabilityRisk, number> = { high: 0, medium: 1, low: 2 };

export function sortCapabilities<T extends { family: string }>(caps: T[]): T[] {
    return [...caps].sort((a, b) => {
        const ra = RISK_ORDER[capabilityMeta(a.family).risk];
        const rb = RISK_ORDER[capabilityMeta(b.family).risk];
        if (ra !== rb) return ra - rb;
        return a.family.localeCompare(b.family);
    });
}
