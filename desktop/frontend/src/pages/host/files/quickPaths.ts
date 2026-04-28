// quickPaths.ts is the platform-aware brain behind the FileBrowser's
// quick-jump chip row. The chip row gives operators a one-click
// teleport to common roots (/, ~, /etc, …) instead of breadcrumb-
// hopping or typing every time. This module returns the chips; the
// QuickPaths component consumes the result.
//
// Pure data, no React, no I/O — so the platform mapping is
// straight-forward to test in isolation. Future Windows/BSD/macOS
// expansions land as contained patches here without touching the
// component.

import type { Host } from "../../../lib/api";

export interface QuickPath {
    // Display label rendered in the chip ("/", "~", "/etc", "C:\\").
    label: string;
    // Absolute path the chip navigates to when clicked.
    path: string;
    // Optional tooltip with extra context.
    title?: string;
}

export function quickPathsForHost(host: Host | null): QuickPath[] | null {
    if (!host) return null;
    if (isWindows(host)) return windowsPaths(host);
    return unixPaths(host);
}

function isWindows(host: Host): boolean {
    const o = (host.os || "").toLowerCase();
    const p = (host.platform || "").toLowerCase();
    return o.includes("windows") || p === "windows" || p.startsWith("win");
}

function unixPaths(host: Host): QuickPath[] {
    const out: QuickPath[] = [{ label: "/", path: "/", title: "Filesystem root" }];
    const home = unixHome(host.current_user);
    if (home) {
        out.push({
            label: "~",
            path: home,
            title: `Home directory (${home})`,
        });
    }
    out.push(
        { label: "/etc", path: "/etc", title: "System configuration" },
        { label: "/var", path: "/var", title: "Variable state (logs, caches)" },
        { label: "/tmp", path: "/tmp", title: "Temporary files" },
    );
    return out;
}

function unixHome(user: string | undefined): string | null {
    if (!user) return null;
    if (user === "root") return "/root";
    return `/home/${user}`;
}

function windowsPaths(host: Host): QuickPath[] {
    const out: QuickPath[] = [
        { label: "C:\\", path: "C:\\", title: "System drive root" },
    ];
    if (host.current_user) {
        const home = `C:\\Users\\${host.current_user}`;
        out.push({ label: "~", path: home, title: `Home directory (${home})` });
    }
    out.push(
        {
            label: "C:\\Windows",
            path: "C:\\Windows",
            title: "Windows installation",
        },
        {
            label: "C:\\ProgramData",
            path: "C:\\ProgramData",
            title: "Application data shared across users",
        },
    );
    return out;
}
