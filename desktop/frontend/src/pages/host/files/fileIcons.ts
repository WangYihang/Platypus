import {
    File,
    FileArchive,
    FileAudio,
    FileCode,
    FileImage,
    FileJson,
    FileSpreadsheet,
    FileSymlink,
    FileText,
    FileType,
    FileVideo,
    Folder,
    Terminal,
} from "lucide-react";
import type { ComponentType } from "react";

import type { FileEntryDTO } from "../../../platform/App.web";

// pickFileIcon picks the lucide icon + tint that best represents an
// entry. Folders, symlinks, and the generic "regular file" fallback
// stay aligned with the previous hard-coded triple — anything that
// matched a known extension before still does — but specific
// extensions now route to a tighter icon (PDF, DOCX, archive, code,
// image, …) so a typical directory becomes scannable at a glance.
//
// We accept an extension-keyed table rather than a MIME-keyed one
// because:
//   · the v2 wire shape includes MIME, but the desktop FileEntryDTO
//     does not always populate it for very old agents — extension is
//     the only universally available signal;
//   · the user's request specifically called out filename suffixes
//     (PDF / DOC), so an explicit ext map is the simplest to extend.

export interface FileIconSpec {
    Icon: ComponentType<{ className?: string }>;
    color: string;
}

const DIR_SPEC: FileIconSpec = { Icon: Folder, color: "text-amber-500" };
const SYMLINK_SPEC: FileIconSpec = { Icon: FileSymlink, color: "text-sky-500" };
const DEFAULT_FILE: FileIconSpec = { Icon: File, color: "text-muted-foreground" };

const ARCHIVE: FileIconSpec = { Icon: FileArchive, color: "text-yellow-600" };
const IMAGE: FileIconSpec = { Icon: FileImage, color: "text-violet-500" };
const VIDEO: FileIconSpec = { Icon: FileVideo, color: "text-pink-500" };
const AUDIO: FileIconSpec = { Icon: FileAudio, color: "text-fuchsia-500" };
const SHELL: FileIconSpec = { Icon: Terminal, color: "text-emerald-500" };

const ICON_BY_EXT: Record<string, FileIconSpec> = {
    // Documents
    pdf: { Icon: FileText, color: "text-red-500" },
    doc: { Icon: FileText, color: "text-blue-500" },
    docx: { Icon: FileText, color: "text-blue-500" },
    rtf: { Icon: FileText, color: "text-blue-500" },
    odt: { Icon: FileText, color: "text-blue-500" },
    pages: { Icon: FileText, color: "text-blue-500" },
    epub: { Icon: FileText, color: "text-emerald-500" },
    mobi: { Icon: FileText, color: "text-emerald-500" },

    // Spreadsheets
    xls: { Icon: FileSpreadsheet, color: "text-emerald-600" },
    xlsx: { Icon: FileSpreadsheet, color: "text-emerald-600" },
    ods: { Icon: FileSpreadsheet, color: "text-emerald-600" },
    csv: { Icon: FileSpreadsheet, color: "text-emerald-600" },
    tsv: { Icon: FileSpreadsheet, color: "text-emerald-600" },
    numbers: { Icon: FileSpreadsheet, color: "text-emerald-600" },

    // Slides
    ppt: { Icon: FileType, color: "text-orange-500" },
    pptx: { Icon: FileType, color: "text-orange-500" },
    odp: { Icon: FileType, color: "text-orange-500" },
    key: { Icon: FileType, color: "text-orange-500" },

    // Plain text / docs
    md: { Icon: FileText, color: "text-sky-500" },
    markdown: { Icon: FileText, color: "text-sky-500" },
    txt: { Icon: FileText, color: "text-muted-foreground" },
    log: { Icon: FileText, color: "text-muted-foreground" },
    rst: { Icon: FileText, color: "text-muted-foreground" },

    // Code — JS / TS family
    js: { Icon: FileCode, color: "text-yellow-500" },
    mjs: { Icon: FileCode, color: "text-yellow-500" },
    cjs: { Icon: FileCode, color: "text-yellow-500" },
    jsx: { Icon: FileCode, color: "text-cyan-500" },
    ts: { Icon: FileCode, color: "text-blue-500" },
    tsx: { Icon: FileCode, color: "text-cyan-500" },

    // Code — other languages
    py: { Icon: FileCode, color: "text-blue-400" },
    rb: { Icon: FileCode, color: "text-red-500" },
    go: { Icon: FileCode, color: "text-cyan-500" },
    rs: { Icon: FileCode, color: "text-orange-600" },
    c: { Icon: FileCode, color: "text-blue-500" },
    h: { Icon: FileCode, color: "text-blue-500" },
    cc: { Icon: FileCode, color: "text-blue-500" },
    cpp: { Icon: FileCode, color: "text-blue-500" },
    hpp: { Icon: FileCode, color: "text-blue-500" },
    hh: { Icon: FileCode, color: "text-blue-500" },
    java: { Icon: FileCode, color: "text-orange-500" },
    kt: { Icon: FileCode, color: "text-purple-500" },
    swift: { Icon: FileCode, color: "text-orange-500" },
    php: { Icon: FileCode, color: "text-indigo-500" },
    cs: { Icon: FileCode, color: "text-violet-500" },
    lua: { Icon: FileCode, color: "text-blue-400" },
    pl: { Icon: FileCode, color: "text-violet-500" },
    r: { Icon: FileCode, color: "text-blue-500" },
    scala: { Icon: FileCode, color: "text-red-500" },
    dart: { Icon: FileCode, color: "text-cyan-500" },
    ex: { Icon: FileCode, color: "text-violet-500" },
    exs: { Icon: FileCode, color: "text-violet-500" },
    erl: { Icon: FileCode, color: "text-red-600" },
    hs: { Icon: FileCode, color: "text-violet-500" },
    clj: { Icon: FileCode, color: "text-emerald-500" },
    nim: { Icon: FileCode, color: "text-yellow-500" },
    zig: { Icon: FileCode, color: "text-amber-500" },

    // Shell / scripts
    sh: SHELL,
    bash: SHELL,
    zsh: SHELL,
    fish: SHELL,
    ksh: SHELL,
    ps1: { Icon: Terminal, color: "text-blue-500" },
    bat: SHELL,
    cmd: SHELL,

    // Web
    html: { Icon: FileCode, color: "text-orange-500" },
    htm: { Icon: FileCode, color: "text-orange-500" },
    css: { Icon: FileCode, color: "text-blue-500" },
    scss: { Icon: FileCode, color: "text-pink-500" },
    sass: { Icon: FileCode, color: "text-pink-500" },
    less: { Icon: FileCode, color: "text-blue-400" },
    vue: { Icon: FileCode, color: "text-emerald-500" },
    svelte: { Icon: FileCode, color: "text-orange-500" },

    // Data / config
    json: { Icon: FileJson, color: "text-amber-500" },
    json5: { Icon: FileJson, color: "text-amber-500" },
    jsonc: { Icon: FileJson, color: "text-amber-500" },
    yaml: { Icon: FileCode, color: "text-rose-500" },
    yml: { Icon: FileCode, color: "text-rose-500" },
    toml: { Icon: FileCode, color: "text-amber-600" },
    ini: { Icon: FileCode, color: "text-muted-foreground" },
    conf: { Icon: FileCode, color: "text-muted-foreground" },
    cfg: { Icon: FileCode, color: "text-muted-foreground" },
    env: { Icon: FileCode, color: "text-emerald-600" },
    xml: { Icon: FileCode, color: "text-orange-500" },
    sql: { Icon: FileCode, color: "text-violet-500" },
    proto: { Icon: FileCode, color: "text-blue-500" },
    graphql: { Icon: FileCode, color: "text-pink-500" },
    gql: { Icon: FileCode, color: "text-pink-500" },

    // Archives
    zip: ARCHIVE,
    tar: ARCHIVE,
    gz: ARCHIVE,
    tgz: ARCHIVE,
    bz2: ARCHIVE,
    xz: ARCHIVE,
    "7z": ARCHIVE,
    rar: ARCHIVE,
    iso: ARCHIVE,
    dmg: ARCHIVE,
    "tar.gz": ARCHIVE,
    "tar.bz2": ARCHIVE,
    "tar.xz": ARCHIVE,

    // Images
    png: IMAGE,
    jpg: IMAGE,
    jpeg: IMAGE,
    gif: IMAGE,
    webp: IMAGE,
    svg: IMAGE,
    bmp: IMAGE,
    ico: IMAGE,
    tif: IMAGE,
    tiff: IMAGE,
    avif: IMAGE,
    heic: IMAGE,
    psd: IMAGE,
    ai: IMAGE,
    sketch: IMAGE,

    // Video
    mp4: VIDEO,
    m4v: VIDEO,
    mov: VIDEO,
    webm: VIDEO,
    mkv: VIDEO,
    avi: VIDEO,
    flv: VIDEO,
    wmv: VIDEO,
    mpg: VIDEO,
    mpeg: VIDEO,

    // Audio
    mp3: AUDIO,
    wav: AUDIO,
    ogg: AUDIO,
    oga: AUDIO,
    flac: AUDIO,
    m4a: AUDIO,
    aac: AUDIO,
    opus: AUDIO,
    wma: AUDIO,

    // Binary / executable
    exe: { Icon: FileCode, color: "text-zinc-500" },
    dll: { Icon: FileCode, color: "text-zinc-500" },
    so: { Icon: FileCode, color: "text-zinc-500" },
    dylib: { Icon: FileCode, color: "text-zinc-500" },
    deb: ARCHIVE,
    rpm: ARCHIVE,
    apk: ARCHIVE,
};

// Compound suffixes need a longest-match probe so ".tar.gz" doesn't
// degrade to plain ".gz". Order matters; check the longer suffix first.
const COMPOUND_EXTS = ["tar.gz", "tar.bz2", "tar.xz"];

export function extKeyOf(name: string): string {
    const lower = name.toLowerCase();
    for (const compound of COMPOUND_EXTS) {
        if (lower.endsWith("." + compound)) return compound;
    }
    const i = lower.lastIndexOf(".");
    // Skip dotfiles (".bashrc" → ext "") and trailing-dot names.
    if (i <= 0 || i === lower.length - 1) return "";
    return lower.slice(i + 1);
}

export function pickFileIcon(entry: FileEntryDTO): FileIconSpec {
    if (entry.isDir) return DIR_SPEC;
    if (entry.isSymlink) return SYMLINK_SPEC;
    const ext = extKeyOf(entry.name);
    return ICON_BY_EXT[ext] ?? DEFAULT_FILE;
}

// isHiddenEntry treats Unix-style dotfiles as hidden. The file
// browser uses this to (a) dim the row/tile and (b) honour the
// "show hidden" toggle in the toolbar.
export function isHiddenEntry(entry: FileEntryDTO): boolean {
    return entry.name.startsWith(".");
}
