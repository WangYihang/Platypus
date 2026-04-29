// Shared OS / arch ordering helpers used by both the inline
// EnrollAgentWizard (Fleet card view) and the legacy IssueInstallDialog
// on the management page (Audit → Enrollment). Lives in its own module
// so the wizard doesn't have to depend on the management-page module
// just to pick up the priority lists.

// Display order for the install-target picker. OSes a deployer is most
// likely to pick come first; within an OS the common 64-bit archs lead
// and the long tail (mips, riscv, …) trails. Anything not in the list
// gets sorted alphabetically and appended — keeps us forward-compatible
// with future GOOS/GOARCH additions without code changes.
export const OS_ORDER = [
    "linux",
    "darwin",
    "windows",
    "freebsd",
    "openbsd",
    "netbsd",
];

export const ARCH_ORDER = [
    "amd64",
    "arm64",
    "arm",
    "386",
    "riscv64",
    "ppc64le",
    "s390x",
    "loong64",
    "mips64le",
    "mips64",
    "mipsle",
    "mips",
];

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
