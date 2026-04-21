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

// basename returns the final segment of a POSIX-style path. Used by the
// Save As dialog suggestion when downloading remote files.
export function basename(p: string): string {
    const i = p.lastIndexOf("/");
    return i >= 0 ? p.slice(i + 1) : p;
}
