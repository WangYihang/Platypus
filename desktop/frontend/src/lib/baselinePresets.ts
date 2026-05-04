// Baseline plugin presets — one-click bundles for the Enroll
// wizard's plugin picker. Operators rarely want to think in terms
// of individual plugins ("sys-listdir, sys-file-read, sys-fs-write,
// sys-file-archive…"); they want to think in terms of intent
// ("I need to inspect this host", "I need to manage files",
// "I need shell + tunnels"). Presets map intent → plugin IDs.
//
// Conventions:
//   · IDs are full reverse-DNS form so they round-trip into the
//     install bundle without any client-side prefixing.
//   · Each preset is a SUBSET of what the system catalog ships.
//     If a preset references a plugin the catalog doesn't have
//     (older server, slimmed publisher run), the wizard
//     gracefully filters it down to what's available.
//   · Presets are intentionally cumulative: "Process control"
//     includes everything in "Read-only inspection" + the process
//     bits, "Full" is every plugin. Operators stop at the lowest
//     tier that lets the host do what they need.

export interface BaselinePreset {
    id: string;
    label: string;
    summary: string;
    pluginIDs: readonly string[];
}

export const BASELINE_PRESETS: BaselinePreset[] = [
    {
        id: "minimal",
        label: "Minimal",
        summary:
            "Host appears in the fleet but exposes no capabilities. Add plugins later from the host's Plugins tab.",
        pluginIDs: [],
    },
    {
        id: "read-only",
        label: "Read-only inspection",
        summary:
            "Browse files, see processes, read system info. No mutations, no shell, no network.",
        pluginIDs: [
            "com.platypus.sys-info",
            "com.platypus.sys-listdir",
            "com.platypus.sys-procs",
            "com.platypus.sys-file-read",
            "com.platypus.sys-file-scan",
        ],
    },
    {
        id: "file-management",
        label: "File management",
        summary:
            "Everything in Read-only plus create / rename / delete files and tar+gz archives. No shell, no network.",
        pluginIDs: [
            "com.platypus.sys-info",
            "com.platypus.sys-listdir",
            "com.platypus.sys-procs",
            "com.platypus.sys-file-read",
            "com.platypus.sys-file-scan",
            "com.platypus.sys-fs-write",
            "com.platypus.sys-file-write",
            "com.platypus.sys-file-archive",
        ],
    },
    {
        id: "operator",
        label: "Operator (file + shell)",
        summary:
            "File management + interactive shell sessions. Use for hosts you actively administer.",
        pluginIDs: [
            "com.platypus.sys-info",
            "com.platypus.sys-listdir",
            "com.platypus.sys-procs",
            "com.platypus.sys-file-read",
            "com.platypus.sys-file-scan",
            "com.platypus.sys-fs-write",
            "com.platypus.sys-file-write",
            "com.platypus.sys-file-archive",
            "com.platypus.sys-exec",
            "com.platypus.sys-process-open",
        ],
    },
    {
        id: "security-audit",
        label: "Security audit",
        summary:
            "Read-only host inspection + the security scan + config audit plugins. Use for compliance / forensics roles.",
        pluginIDs: [
            "com.platypus.sys-info",
            "com.platypus.sys-listdir",
            "com.platypus.sys-procs",
            "com.platypus.sys-file-read",
            "com.platypus.sys-file-scan",
            "com.platypus.sys-security",
            "com.platypus.sys-config-audit",
        ],
    },
    {
        id: "full",
        label: "Full",
        summary:
            "Every system plugin the catalog ships. Equivalent to every checkbox below ticked. Higher trust surface — pick a tighter preset when the role allows it.",
        pluginIDs: [
            "com.platypus.sys-info",
            "com.platypus.sys-listdir",
            "com.platypus.sys-procs",
            "com.platypus.sys-file-read",
            "com.platypus.sys-file-scan",
            "com.platypus.sys-fs-write",
            "com.platypus.sys-file-write",
            "com.platypus.sys-file-archive",
            "com.platypus.sys-exec",
            "com.platypus.sys-process-open",
            "com.platypus.sys-security",
            "com.platypus.sys-config-audit",
            "com.platypus.sys-tunnel-pull",
        ],
    },
];

// matchingPreset returns the preset whose pluginIDs (restricted to
// the catalog actually offers) exactly equal the operator's
// current selection. Used for visual highlighting — once they tick
// individual rows that don't add up to a preset, no card lights up.
export function matchingPreset(
    selected: string[],
    catalogIDs: Set<string>,
): BaselinePreset | null {
    const sel = new Set(selected);
    for (const p of BASELINE_PRESETS) {
        const expected = new Set(p.pluginIDs.filter((id) => catalogIDs.has(id)));
        if (expected.size !== sel.size) continue;
        let allMatch = true;
        for (const id of expected) {
            if (!sel.has(id)) {
                allMatch = false;
                break;
            }
        }
        if (allMatch) return p;
    }
    return null;
}
