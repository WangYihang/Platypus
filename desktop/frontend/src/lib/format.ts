// humanize renders a byte count as "123 KiB" with binary units. Used by
// the Files panel's size readout; extracted so later pages (host details,
// downloads) can share it without copy-paste.
export function humanize(n: number): string {
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    let v = n;
    let unit = 0;
    while (v >= 1024 && unit < units.length - 1) {
        v /= 1024;
        unit++;
    }
    return `${v.toFixed(unit === 0 ? 0 : 2)} ${units[unit]}`;
}

// formatBytes is the status-bar version of humanize — same binary
// units but only one decimal place so the "47.0 MiB" pill stays
// readable. Returns "—" for nullish input so the StatusBar can
// splice the result into a JSX cell without a guard.
export function formatBytes(n: number | null | undefined): string {
    if (n === null || n === undefined || !Number.isFinite(n)) return "—";
    if (n < 0) return "—";
    if (n === 0) return "0 B";
    if (n < 1024) return `${Math.round(n)} B`;
    const units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
    let v = n / 1024;
    let unit = 0;
    while (v >= 1024 && unit < units.length - 1) {
        v /= 1024;
        unit++;
    }
    return `${v.toFixed(1)} ${units[unit]}`;
}

// formatUptimeSeconds renders a process uptime in the two most-
// significant units: "5s", "2m 5s", "2h 30m", "3d 4h". The cap
// keeps the status-bar pill terse — operators who need the exact
// number can hover for the title= or look at started_at_unix.
export function formatUptimeSeconds(secs: number | null | undefined): string {
    if (secs === null || secs === undefined || !Number.isFinite(secs)) return "—";
    if (secs < 0) return "—";
    const n = Math.floor(secs);
    if (n < 60) return `${n}s`;
    if (n < 3600) {
        const m = Math.floor(n / 60);
        const s = n % 60;
        return s === 0 ? `${m}m` : `${m}m ${s}s`;
    }
    if (n < 86400) {
        const h = Math.floor(n / 3600);
        const m = Math.floor((n % 3600) / 60);
        return m === 0 ? `${h}h` : `${h}h ${m}m`;
    }
    const d = Math.floor(n / 86400);
    const h = Math.floor((n % 86400) / 3600);
    return h === 0 ? `${d}d` : `${d}d ${h}h`;
}

// basename returns the final segment of a POSIX-style path. Used by the
// Save As dialog suggestion when downloading remote files.
export function basename(p: string): string {
    const i = p.lastIndexOf("/");
    return i >= 0 ? p.slice(i + 1) : p;
}
