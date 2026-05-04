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
    | "net.http"
    | "net.dial"
    | "process";

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
    process: {
        family: "process",
        label: "Spawn processes (streaming)",
        risk: "high",
        summary:
            "Spawn an interactive child process (PTY-backed shells, long-lived daemons). Higher blast radius than `exec` because the operator's stdin reaches the child over the network.",
    },
    "net.dial": {
        family: "net.dial",
        label: "Outbound TCP dial",
        risk: "high",
        summary:
            "Open a raw TCP connection to the declared targets and splice bytes bidirectionally. Effectively SSRF authority if granted to a wildcard target — review the targets list carefully before approving.",
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

// --- Capability collections -----------------------------------------
//
// Operators almost never want to grant individual primitives — they
// want a coherent group ("can read files", "can run commands +
// processes"). Collections are presets that map a single click to
// the underlying capability families.
//
// Design intent:
//   · Five tiers ordered by power: read-only → file mgmt → process →
//     network → full. Operators pick the lowest tier that still lets
//     the plugin do its job.
//   · Each collection is OPEN — operators can still tick individual
//     families afterwards; selecting a collection just pre-fills
//     them. Likewise, deselecting a collection unticks its members.
//   · "Logging" is granted by every collection (and implicitly by
//     the agent for every plugin). It's not surfaced as a separate
//     toggle since that would be noise.
//   · The "Custom" path — picking individual families with no
//     collection selected — is always available; collections are a
//     convenience layer, not a constraint.

export type CollectionID = "read-only" | "file-management" | "process" | "network" | "full";

export interface CapabilityCollection {
    id: CollectionID;
    label: string;
    summary: string;
    risk: CapabilityRisk;
    /** Capability families auto-granted when this collection is picked. */
    families: CapabilityFamily[];
}

export const CAPABILITY_COLLECTIONS: CapabilityCollection[] = [
    {
        id: "read-only",
        label: "Read-only inspection",
        summary:
            "Read files and report system metadata. Safe for monitoring / audit-style plugins. No mutations, no command execution, no network.",
        risk: "low",
        families: ["log", "sysinfo", "fs.read", "kv"],
    },
    {
        id: "file-management",
        label: "File management",
        summary:
            "Read AND write files within the declared paths. Use for sync / backup / config-deploy plugins.",
        risk: "medium",
        families: ["log", "sysinfo", "fs.read", "fs.write", "kv"],
    },
    {
        id: "process",
        label: "Process control",
        summary:
            "Read files + spawn declared commands + open interactive processes. Use for shell / orchestration plugins.",
        risk: "high",
        families: ["log", "sysinfo", "fs.read", "exec", "process", "kv"],
    },
    {
        id: "network",
        label: "Network access",
        summary:
            "Read files + reach declared HTTP hosts + open raw TCP. Use for plugins that integrate with remote services.",
        risk: "high",
        families: ["log", "sysinfo", "fs.read", "net.http", "net.dial", "kv"],
    },
    {
        id: "full",
        label: "Full access",
        summary:
            "Every capability the plugin's manifest declares. Equivalent to ticking every box. Only grant when the plugin's docs require it.",
        risk: "high",
        families: ["log", "sysinfo", "fs.read", "fs.write", "exec", "process", "net.http", "net.dial", "kv"],
    },
];

export function collectionByID(id: string): CapabilityCollection | undefined {
    return CAPABILITY_COLLECTIONS.find((c) => c.id === id);
}

// matchingCollection returns the highest-coverage collection that's
// fully satisfied by `granted` AND whose families are all declared
// (we can't claim "process" if the plugin doesn't even declare exec).
// Returns null when no collection matches — i.e. the operator's set
// is a custom mix.
//
// "Highest-coverage" = the longest families[] of the matching ones,
// so picking { fs.read, fs.write } highlights "file-management"
// rather than "read-only".
export function matchingCollection(
    declared: Set<string>,
    granted: Set<string>,
): CapabilityCollection | null {
    let best: CapabilityCollection | null = null;
    for (const c of CAPABILITY_COLLECTIONS) {
        // The collection's intent must be fully covered by `granted`,
        // restricted to the families this plugin actually declares.
        const expected = c.families.filter((f) => declared.has(f));
        if (expected.length === 0) continue;
        const allCovered = expected.every((f) => granted.has(f));
        if (!allCovered) continue;
        // No granted family outside the collection's authority.
        const noExtras = [...granted].every((f) => c.families.includes(f as CapabilityFamily) || !declared.has(f));
        if (!noExtras) continue;
        if (best === null || expected.length > best.families.filter((f) => declared.has(f)).length) {
            best = c;
        }
    }
    return best;
}
