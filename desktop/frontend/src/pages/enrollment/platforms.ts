// Shared OS / arch ordering helpers used by both the inline
// EnrollAgentWizard (Fleet card view) and the legacy IssueInstallDialog
// on the management page (Audit → Enrollment). Lives in its own module
// so the wizard doesn't have to depend on the management-page module
// just to pick up the priority lists.

import { InstallPlatform } from "../../lib/api";

// PlatformsState tracks the install-target picker's lifecycle so the
// UI can disable the dropdown while loading and surface the right
// empty / error hint without conflating "no manifest published" with
// "request failed". Same shape used by the management-page picker
// and the four-step wizard.
export type PlatformsState =
    | { status: "loading" }
    | { status: "ready"; platforms: InstallPlatform[]; channel: string }
    | { status: "empty"; channel: string }
    | { status: "error"; message: string };

// Display order for the install-target picker. OSes a deployer is most
// likely to pick come first; the long tail (plan9, illumos, …) trails.
// Anything not in the list gets sorted alphabetically and appended —
// keeps us forward-compatible with future GOOS additions without code
// changes. The full list mirrors `go tool dist list`'s GOOS column so
// any binary the release pipeline publishes lands in a sensible slot.
export const OS_ORDER = [
    "linux",
    "darwin",
    "windows",
    "android",
    "ios",
    "freebsd",
    "openbsd",
    "netbsd",
    "dragonfly",
    "solaris",
    "illumos",
    "aix",
    "plan9",
    "js",
    "wasip1",
];

// Same idea for GOARCH. amd64 / arm64 lead because that's >95% of real
// installs; the long tail trails. Keep `wasm` near `js`/`wasip1`'s
// neighbourhood since it's the only arch they pair with.
export const ARCH_ORDER = [
    "amd64",
    "arm64",
    "arm",
    "386",
    "riscv64",
    "ppc64le",
    "ppc64",
    "s390x",
    "loong64",
    "mips64le",
    "mips64",
    "mipsle",
    "mips",
    "wasm",
];

// Human-readable OS labels for the picker. We intentionally keep the
// raw GOOS as the *value* (it's what the manifest and the install
// endpoint key on) and only humanize the visible label. Anything
// missing from the map falls back to the GOOS string verbatim, so an
// unknown future OS still renders something instead of a blank chip.
export const OS_LABELS: Record<string, string> = {
    linux: "Linux",
    darwin: "macOS",
    windows: "Windows",
    android: "Android",
    ios: "iOS",
    freebsd: "FreeBSD",
    openbsd: "OpenBSD",
    netbsd: "NetBSD",
    dragonfly: "DragonFly BSD",
    solaris: "Solaris",
    illumos: "illumos",
    aix: "AIX",
    plan9: "Plan 9",
    js: "JS (browser)",
    wasip1: "WASI",
};

// Human-readable arch labels. The values stay GOARCH; the labels add
// the colloquial names operators tend to recognise faster than the
// GOARCH string ("Apple Silicon" reads better than "arm64" on the
// macOS row, for example — but the arch picker is OS-agnostic so we
// stick to the generic CPU name and let the OS-specific aliases live
// in the quick-pick presets instead).
export const ARCH_LABELS: Record<string, string> = {
    amd64: "x86_64 (amd64)",
    arm64: "ARM64",
    arm: "ARM (32-bit)",
    "386": "x86 (32-bit)",
    riscv64: "RISC-V 64",
    ppc64le: "PowerPC 64 LE",
    ppc64: "PowerPC 64",
    s390x: "IBM Z (s390x)",
    loong64: "LoongArch 64",
    mips64le: "MIPS64 LE",
    mips64: "MIPS64",
    mipsle: "MIPS LE",
    mips: "MIPS",
    wasm: "WebAssembly",
};

export function osLabel(os: string): string {
    return OS_LABELS[os] ?? os;
}

export function archLabel(arch: string): string {
    return ARCH_LABELS[arch] ?? arch;
}

// preferredOrder returns a comparator that ranks `priority` items first
// (in their declared order) and trails everything else alphabetically.
// Used to bubble the densest-used OSes / archs to the front of the
// pickers without dropping forward compatibility for new GOOS/GOARCH
// values that ship in future manifests.
export function preferredOrder(priority: string[]): (a: string, b: string) => number {
    return (a, b) => {
        const ia = priority.indexOf(a);
        const ib = priority.indexOf(b);
        if (ia === -1 && ib === -1) return a.localeCompare(b);
        if (ia === -1) return 1;
        if (ib === -1) return -1;
        return ia - ib;
    };
}

// One-click "Quick start" presets used to live here as a static
// table. They've moved to project-scoped, persisted EnrollmentPresets
// (see lib/api/enrollment_presets.ts and the wizard's pick_preset
// step) so operators own and can edit them. Removing the static
// definitions also removes the only place in the FE that used to
// hard-code Linux/Windows/macOS labels — keep that property if you're
// adding new platform handling here.
