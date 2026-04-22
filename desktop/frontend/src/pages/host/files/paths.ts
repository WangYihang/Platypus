// POSIX-ish path helpers. We deliberately don't pull in `path-browserify`
// — the agent is almost always *nix, and Windows agents are addressed
// with forward-slash paths through this UI anyway.

export function joinPath(dir: string, name: string): string {
    if (!dir || dir === "/") return "/" + name;
    if (dir.endsWith("/")) return dir + name;
    return dir + "/" + name;
}

export function parentPath(p: string): string {
    if (!p || p === "/") return "/";
    const trimmed = p.replace(/\/+$/, "");
    const i = trimmed.lastIndexOf("/");
    if (i <= 0) return "/";
    return trimmed.slice(0, i);
}

export function basePath(p: string): string {
    if (!p) return "";
    const trimmed = p.replace(/\/+$/, "");
    const i = trimmed.lastIndexOf("/");
    return i >= 0 ? trimmed.slice(i + 1) : trimmed;
}

// splitCrumbs turns "/etc/nginx/conf.d" into:
//   [{ label: "/", path: "/" },
//    { label: "etc", path: "/etc" },
//    { label: "nginx", path: "/etc/nginx" },
//    { label: "conf.d", path: "/etc/nginx/conf.d" }]
// The root segment is always emitted so the breadcrumb can anchor home.
export function splitCrumbs(p: string): { label: string; path: string }[] {
    const crumbs: { label: string; path: string }[] = [{ label: "/", path: "/" }];
    if (!p || p === "/") return crumbs;
    const parts = p.split("/").filter(Boolean);
    let acc = "";
    for (const part of parts) {
        acc += "/" + part;
        crumbs.push({ label: part, path: acc });
    }
    return crumbs;
}

// formatMode renders a unix mode uint32 like "drwxr-xr-x". Mirrors
// FileEntry.mode which is os.FileMode bits — so bit 0x80000000 means
// directory, etc. We only look at the low 9 bits for permissions.
export function formatMode(mode: number, isDir: boolean, isSymlink: boolean): string {
    let head = "-";
    if (isDir) head = "d";
    else if (isSymlink) head = "l";
    const b = (shift: number, ch: string) => ((mode >> shift) & 1 ? ch : "-");
    return (
        head +
        b(8, "r") +
        b(7, "w") +
        b(6, "x") +
        b(5, "r") +
        b(4, "w") +
        b(3, "x") +
        b(2, "r") +
        b(1, "w") +
        b(0, "x")
    );
}

// formatModeOctal returns the 4-digit octal perm string ("0644") — what
// the Chmod dialog and REST API expect.
export function formatModeOctal(mode: number): string {
    return (mode & 0o7777).toString(8).padStart(3, "0");
}

// inferLanguage maps a filename to a CodeMirror language key. Keep the
// mapping lazy-loadable in FileEditor — this only returns the key.
export function inferLanguage(name: string): "json" | "javascript" | "python" | "shell" | "plain" {
    const lower = name.toLowerCase();
    if (lower.endsWith(".json")) return "json";
    if (/\.(js|jsx|ts|tsx|mjs|cjs)$/.test(lower)) return "javascript";
    if (/\.(py|pyi)$/.test(lower)) return "python";
    if (/\.(sh|bash|zsh|fish)$/.test(lower) || lower === ".bashrc" || lower === ".zshrc") {
        return "shell";
    }
    return "plain";
}
