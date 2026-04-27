// Web-mode backend for the @wails/go/app/App alias. Every name the pages
// import from that alias is re-exported here with a REST/WebSocket
// implementation that talks to platypus-server directly. tsconfig.json
// also points the alias here unconditionally so `tsc` type-checks both
// modes against this signature surface. The server's CORS config is `*`
// (see internal/api/rest.go) so cross-origin fetch + WebSocket work.
//
// Auth comes from lib/auth — the JWT session set up by Login.tsx. The
// in-memory access token there is the source of truth; this shim does
// NOT manage its own token store any more (it used to, back when the
// server only spoke the legacy single-secret /auth/token flow).

import { authFetch, authJSON, getSession } from "../lib/auth";
import { emitEvent } from "./runtime.web";

// ---------- HTTP helpers ------------------------------------------------

async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
    return authFetch(path, init);
}
async function apiJSON<T>(path: string, init?: RequestInit): Promise<T> {
    return authJSON<T>(path, init);
}

function getServerURL(): string {
    const s = getSession();
    if (!s) throw new Error("not connected — log in first");
    return s.serverURL;
}

// ---------- Files -------------------------------------------------------
// Pages still call PickFileToUpload → upload path → UploadFile(path). In a
// browser there are no real paths, so we stash the user-picked File in a
// Map keyed by a synthetic "web-file://N" token and return that token as
// the path. UploadFile reads the bytes back out and chunks them to the
// server. Downloads trigger a browser download via a blob URL.

let uploadCounter = 0;
const pendingUploads = new Map<string, File>();
const pendingDownloadNames = new Map<string, string>();

function pickFile(): Promise<File | null> {
    return new Promise((resolve) => {
        const input = document.createElement("input");
        input.type = "file";
        input.style.display = "none";
        document.body.appendChild(input);
        input.addEventListener("change", () => {
            const f = input.files?.[0] ?? null;
            document.body.removeChild(input);
            resolve(f);
        });
        // Firing Cancel on a native file dialog doesn't fire a change event
        // on older browsers, leaving the promise unresolved. window.focus
        // after the picker closes is a heuristic for detecting cancel.
        input.addEventListener("cancel", () => {
            document.body.removeChild(input);
            resolve(null);
        });
        input.click();
    });
}

export async function PickFileToUpload(_title?: string): Promise<string> {
    const f = await pickFile();
    if (!f) return "";
    const id = `web-file://${++uploadCounter}`;
    pendingUploads.set(id, f);
    return id;
}

export async function PickSaveLocation(_title: string, defaultName: string): Promise<string> {
    // Browsers don't expose save-as dialogs. Just stash the requested name —
    // DownloadFile uses it as the filename hint for the <a download> trigger.
    const id = `web-dl://${++uploadCounter}`;
    pendingDownloadNames.set(id, defaultName || "download.bin");
    return id;
}

// PickDirectoryToSave is the desktop directory picker; in web mode we
// have no native equivalent that's available across browsers (the File
// System Access API is gated behind a user gesture and unsupported in
// Firefox/Safari). The shim returns a sentinel "browser-dl-dir://"
// path; Download* helpers downstream see the prefix and fall back to
// per-file <a download> triggers, letting the browser write each file
// into the user's default Downloads folder.
export async function PickDirectoryToSave(_title?: string): Promise<string> {
    return "browser-dl-dir://default";
}

// All file operations target the v2 agent RPC surface. `sessionHash` is
// carried as the legacy parameter name but its value is actually the v2
// agent_id (which is also the session id — see internal/core.AgentLinkService).
//
// v2 REST contract (internal/api/handler_file_v2.go + handler_rpc_v2.go),
// project-scoped under /api/v1/projects/:pid/agents/:agent_id/...:
//   GET    .../fs/read?path=&offset=&length=   → octet-stream
//   PUT    .../fs/write?path=&append=&mkdirs=  → { bytes_written }
//   GET    .../fs/list?path=                   → { entries }
//   GET    .../fs/stat?path=                   → { entry }
//   DELETE .../fs/remove?path=&recursive=      → 200 or 502
//   POST   .../fs/rename?from=&to=             → 200 or 502
//   POST   .../fs/mkdir?path=&mkdirs=&mode=    → 200 or 502
//   PATCH  .../fs/mode?path=&mode=             → 200 or 502
// Errors surface as HTTP 4xx/5xx bodies; authFetch throws those automatically.
//
// projectID gates RBAC server-side: a viewer of pid A can't read files on
// an agent in pid B even if they know its agent_id (handler_file_v2.go +
// rbac.RequireAgentInProject). Callers always thread Project.id from
// useShell() into these helpers; never hard-code or guess it.

function fsURL(projectID: string, agentID: string, suffix: string): string {
    return `/api/v1/projects/${encodeURIComponent(projectID)}/agents/${encodeURIComponent(agentID)}/fs/${suffix}`;
}

export async function FileSize(projectID: string, sessionHash: string, path: string): Promise<number> {
    const entry = await StatFile(projectID, sessionHash, path);
    return entry.size || 0;
}

const CHUNK_SIZE = 256 * 1024;

export async function ReadFile(
    projectID: string,
    sessionHash: string,
    path: string,
    offset: number,
    size: number,
): Promise<number[]> {
    const q = new URLSearchParams({
        path,
        offset: String(offset),
        length: String(size),
    });
    const r = await apiFetch(fsURL(projectID, sessionHash, `read?${q}`));
    const buf = new Uint8Array(await r.arrayBuffer());
    return Array.from(buf);
}

export async function WriteFile(
    projectID: string,
    sessionHash: string,
    path: string,
    data: number[],
    appendMode: boolean,
): Promise<void> {
    const q = new URLSearchParams({ path, append: String(appendMode) });
    await apiFetch(fsURL(projectID, sessionHash, `write?${q}`), {
        method: "PUT",
        headers: { "Content-Type": "application/octet-stream" },
        body: new Uint8Array(data),
    });
}

export async function DownloadFile(
    projectID: string,
    sessionHash: string,
    remotePath: string,
    localPath: string,
): Promise<void> {
    const name = pendingDownloadNames.get(localPath) || "download.bin";
    pendingDownloadNames.delete(localPath);

    // v2 fs/read streams the whole file in a single HTTP response when
    // offset=0 & length=0 — the server sets Content-Length on full
    // downloads so the browser gets a proper progress bar.
    const q = new URLSearchParams({ path: remotePath, offset: "0", length: "0" });
    const r = await apiFetch(fsURL(projectID, sessionHash, `read?${q}`));
    const blob = await r.blob();

    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    // Revoke lazily — some browsers fire the download after the click handler returns.
    setTimeout(() => URL.revokeObjectURL(url), 1_000);
}

// DownloadFolder walks the remote tree and triggers a browser <a
// download> per file. Kept for backwards compatibility with the few
// callers that still want one-file-per-download — the new archive
// path (DownloadArchive) is preferred for any folder selection.
export async function DownloadFolder(
    projectID: string,
    sessionHash: string,
    remotePath: string,
    _localDir: string,
): Promise<void> {
    const root = remotePath.replace(/\/+$/, "") || "/";
    const stack: string[] = [root];
    while (stack.length > 0) {
        const dir = stack.pop() as string;
        const list = await ListDir(projectID, sessionHash, dir, 0, 0);
        for (const entry of list.entries) {
            if (entry.error) continue;
            if (entry.isSymlink) continue;
            const child = dir.endsWith("/") ? `${dir}${entry.name}` : `${dir}/${entry.name}`;
            if (entry.isDir) {
                stack.push(child);
                continue;
            }
            // Trigger a browser download with the file's leaf name.
            const id = `web-dl://${++uploadCounter}`;
            pendingDownloadNames.set(id, entry.name);
            await DownloadFile(projectID, sessionHash, child, id);
        }
    }
}

// DownloadArchive packages remote paths into a single archive and
// hands it off as a browser download. The format param matches the
// desktop App.go binding so call sites are mode-agnostic.
//
// Web-mode caveat: the browser doesn't ship a streaming tar/gzip/zip
// encoder we can rely on without a dependency, so this shim builds a
// plain (uncompressed) tar regardless of the requested format. Reads
// stay chunked at CHUNK_SIZE so the per-call payload matches every
// other endpoint and the agent never gets asked for the whole file
// at once. The full archive does end up in browser memory before
// download — a future replacement can stream via StreamSaver or the
// File System Access API once we want web mode to handle multi-GB
// archives.
export async function DownloadArchive(
    projectID: string,
    sessionHash: string,
    remotePaths: string[],
    localPath: string,
    _format: string,
): Promise<void> {
    if (remotePaths.length === 0) {
        throw new Error("DownloadArchive: empty selection");
    }
    const filename = pendingDownloadNames.get(localPath) || "archive.tar";
    pendingDownloadNames.delete(localPath);

    // chunks accumulates the tar bytes as separate Uint8Arrays so we
    // never realloc a single growing buffer.
    const chunks: Uint8Array[] = [];
    for (const root of remotePaths) {
        const stat = await StatFile(projectID, sessionHash, root);
        const base = baseName(root) || "root";
        await streamTarEntry(projectID, sessionHash, root, base, stat, chunks);
    }
    // tar end-of-archive marker: two zero blocks.
    chunks.push(new Uint8Array(TAR_BLOCK * 2));

    // Cast through BlobPart[] — TS's strict ArrayBuffer / SharedArrayBuffer
    // distinction in lib.dom.d.ts isn't worth fighting for a runtime
    // value Blob accepts trivially.
    const blob = new Blob(chunks as unknown as BlobPart[], {
        type: "application/x-tar",
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename.endsWith(".tar") ? filename : `${filename}.tar`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 1_000);
}

// --- minimal tar encoder (uncompressed, ustar). ---------------------
// Used only by the web-mode DownloadArchive above; the desktop path
// uses Go's archive/tar. Kept intentionally tiny — no symlinks, no
// PAX, no UID/GID/UNAME — the archive is meant for "download these
// files", not "preserve every Unix detail".

const TAR_BLOCK = 512;

async function streamTarEntry(
    projectID: string,
    sessionHash: string,
    remotePath: string,
    archivePath: string,
    stat: FileEntryDTO,
    out: Uint8Array[],
): Promise<void> {
    if (stat.isDir) {
        out.push(tarHeader(archivePath + "/", 0, stat.mode, stat.modTimeUnix, true));
        const list = await ListDir(projectID, sessionHash, remotePath, 0, 0);
        for (const e of list.entries) {
            if (e.error || e.isSymlink) continue;
            if (!safeChildName(e.name)) continue;
            const child = remotePath.endsWith("/")
                ? `${remotePath}${e.name}`
                : `${remotePath}/${e.name}`;
            await streamTarEntry(
                projectID,
                sessionHash,
                child,
                `${archivePath}/${e.name}`,
                e,
                out,
            );
        }
        return;
    }
    out.push(tarHeader(archivePath, stat.size, stat.mode, stat.modTimeUnix, false));
    let off = 0;
    while (off < stat.size) {
        const want = Math.min(CHUNK_SIZE, stat.size - off);
        const chunkArr = await ReadFile(projectID, sessionHash, remotePath, off, want);
        if (!chunkArr || chunkArr.length === 0) {
            throw new Error(`DownloadArchive: empty chunk for ${remotePath} @${off}`);
        }
        const chunk = new Uint8Array(chunkArr);
        out.push(chunk);
        off += chunk.length;
    }
    // Pad to 512-byte boundary.
    const pad = (TAR_BLOCK - (stat.size % TAR_BLOCK)) % TAR_BLOCK;
    if (pad > 0) out.push(new Uint8Array(pad));
}

function tarHeader(
    name: string,
    size: number,
    mode: number,
    mtimeUnix: number,
    isDir: boolean,
): Uint8Array {
    const h = new Uint8Array(TAR_BLOCK);
    writeASCII(h, 0, name, 100);
    writeOctal(h, 100, mode & 0o7777, 8);
    writeOctal(h, 108, 0, 8); // uid
    writeOctal(h, 116, 0, 8); // gid
    writeOctal(h, 124, isDir ? 0 : size, 12);
    writeOctal(h, 136, mtimeUnix > 0 ? Math.floor(mtimeUnix) : Math.floor(Date.now() / 1000), 12);
    // Checksum: pre-fill the field with 8 spaces, sum every byte,
    // then write the octal sum back.
    for (let i = 148; i < 156; i++) h[i] = 0x20;
    h[156] = isDir ? 0x35 /* '5' */ : 0x30 /* '0' */;
    writeASCII(h, 257, "ustar\0", 6);
    writeASCII(h, 263, "00", 2);
    let sum = 0;
    for (let i = 0; i < TAR_BLOCK; i++) sum += h[i];
    const cs = sum.toString(8).padStart(6, "0") + "\0 ";
    writeASCII(h, 148, cs, 8);
    return h;
}

function writeASCII(buf: Uint8Array, off: number, s: string, len: number) {
    const enc = new TextEncoder().encode(s);
    for (let i = 0; i < len; i++) {
        buf[off + i] = i < enc.length ? enc[i] : 0;
    }
}

function writeOctal(buf: Uint8Array, off: number, n: number, len: number) {
    // POSIX tar: null-terminated octal ASCII, left-padded with zeros.
    const s = n.toString(8);
    const pad = len - 1 - s.length;
    for (let i = 0; i < pad; i++) buf[off + i] = 0x30; // '0'
    for (let i = 0; i < s.length; i++) buf[off + pad + i] = s.charCodeAt(i);
    buf[off + len - 1] = 0;
}

function baseName(p: string): string {
    const trimmed = p.replace(/\/+$/, "");
    const slash = trimmed.lastIndexOf("/");
    return slash === -1 ? trimmed : trimmed.slice(slash + 1);
}

function safeChildName(n: string): boolean {
    if (n === "" || n === "." || n === "..") return false;
    if (n.includes("/") || n.includes("\\")) return false;
    return true;
}

export async function UploadFile(
    projectID: string,
    sessionHash: string,
    remotePath: string,
    localPath: string,
): Promise<void> {
    const f = pendingUploads.get(localPath);
    if (!f) throw new Error(`no pending upload for ${localPath} — did PickFileToUpload run?`);
    pendingUploads.delete(localPath);

    const bytes = new Uint8Array(await f.arrayBuffer());
    // Empty file: truncate the destination with an empty payload.
    if (bytes.length === 0) {
        const q = new URLSearchParams({ path: remotePath, append: "false" });
        await apiFetch(fsURL(projectID, sessionHash, `write?${q}`), {
            method: "PUT",
            headers: { "Content-Type": "application/octet-stream" },
            body: new Uint8Array(0),
        });
        return;
    }
    // First chunk truncates, rest append — matches Go WriteFile semantics.
    for (let off = 0; off < bytes.length; off += CHUNK_SIZE) {
        const slice = bytes.subarray(off, Math.min(off + CHUNK_SIZE, bytes.length));
        const q = new URLSearchParams({
            path: remotePath,
            append: String(off > 0),
        });
        await apiFetch(fsURL(projectID, sessionHash, `write?${q}`), {
            method: "PUT",
            headers: { "Content-Type": "application/octet-stream" },
            body: slice,
        });
    }
}

// ---------- File management (list / stat / delete / rename / mkdir / chmod)
// These mirror the Wails-side App methods with the same signatures. The
// UI imports them from the platform barrel, so switching desktop ↔ web is
// transparent.

export interface FileEntryDTO {
    name: string;
    size: number;
    mode: number;
    modTimeUnix: number;
    isDir: boolean;
    isSymlink: boolean;
    symlinkTarget?: string;
    error?: string;
}

export interface ListDirResult {
    entries: FileEntryDTO[];
    total: number;
    eof: boolean;
}

// v2 wire shape for FileEntry (proto json tags from pkg/proto/v2/rpc.proto).
// Directory / symlink type bits are encoded in the Go FileMode; we derive
// the booleans the UI expects here so existing call sites keep working.
interface V2FileEntry {
    name: string;
    mode: number;
    size: number;
    mtime_unix_nano: number;
    symlink_target?: string;
}

// Go os.FileMode type bits (see src/os/types.go in the Go source):
//   ModeDir      = 1 << 31
//   ModeSymlink  = 1 << 27
const GO_MODE_DIR = 1 << 31;
const GO_MODE_SYMLINK = 1 << 27;

function adaptEntry(e: V2FileEntry): FileEntryDTO {
    return {
        name: e.name,
        size: e.size ?? 0,
        mode: e.mode ?? 0,
        modTimeUnix: e.mtime_unix_nano ?? 0,
        isDir: ((e.mode ?? 0) & GO_MODE_DIR) !== 0,
        isSymlink: ((e.mode ?? 0) & GO_MODE_SYMLINK) !== 0,
        symlinkTarget: e.symlink_target,
    };
}

export async function ListDir(
    projectID: string,
    sessionHash: string,
    path: string,
    _offset: number,
    _limit: number,
): Promise<ListDirResult> {
    // v2 fs/list returns the full directory in a single call; offset/limit
    // are ignored server-side. The desktop signature is kept so callers
    // don't need to branch.
    const q = new URLSearchParams({ path });
    const resp = await apiJSON<{ entries?: V2FileEntry[] }>(
        fsURL(projectID, sessionHash, `list?${q}`),
    );
    const entries = (resp.entries || []).map(adaptEntry);
    return { entries, total: entries.length, eof: true };
}

export async function StatFile(projectID: string, sessionHash: string, path: string): Promise<FileEntryDTO> {
    const q = new URLSearchParams({ path });
    const resp = await apiJSON<{ entry?: V2FileEntry }>(
        fsURL(projectID, sessionHash, `stat?${q}`),
    );
    if (!resp.entry) throw new Error("stat: empty entry");
    return adaptEntry(resp.entry);
}

export async function DeleteFile(
    projectID: string,
    sessionHash: string,
    path: string,
    recursive: boolean,
): Promise<void> {
    const q = new URLSearchParams({ path, recursive: String(recursive) });
    await apiFetch(fsURL(projectID, sessionHash, `remove?${q}`), { method: "DELETE" });
}

export async function RenameFile(projectID: string, sessionHash: string, from: string, to: string): Promise<void> {
    const q = new URLSearchParams({ from, to });
    await apiFetch(fsURL(projectID, sessionHash, `rename?${q}`), { method: "POST" });
}

export async function Mkdir(
    projectID: string,
    sessionHash: string,
    path: string,
    parents: boolean,
    mode: number,
): Promise<void> {
    // v2 takes decimal mode (strconv.ParseUint base 10); callers pass a
    // numeric FileMode like 0o755, which stringifies to "493" — correct.
    const q = new URLSearchParams({ path, mkdirs: String(parents) });
    if (mode && mode !== 0) q.set("mode", String(mode));
    await apiFetch(fsURL(projectID, sessionHash, `mkdir?${q}`), { method: "POST" });
}

export async function Chmod(projectID: string, sessionHash: string, path: string, mode: number): Promise<void> {
    const q = new URLSearchParams({ path, mode: String(mode) });
    await apiFetch(fsURL(projectID, sessionHash, `mode?${q}`), { method: "PATCH" });
}

// UploadBrowserFile streams a File object directly (no synthetic token
// indirection), matching the signature the Wails binding produces for
// OS-drop callbacks. The React drop zone iterates dataTransfer.files and
// calls this per file.
export async function UploadBrowserFile(
    projectID: string,
    sessionHash: string,
    remotePath: string,
    file: File,
): Promise<void> {
    const bytes = new Uint8Array(await file.arrayBuffer());
    if (bytes.length === 0) {
        // Empty file: truncate the destination with an empty payload.
        const q = new URLSearchParams({ path: remotePath, append: "false" });
        await apiFetch(fsURL(projectID, sessionHash, `write?${q}`), {
            method: "PUT",
            headers: { "Content-Type": "application/octet-stream" },
            body: new Uint8Array(0),
        });
        return;
    }
    for (let off = 0; off < bytes.length; off += CHUNK_SIZE) {
        const slice = bytes.subarray(off, Math.min(off + CHUNK_SIZE, bytes.length));
        const q = new URLSearchParams({
            path: remotePath,
            append: String(off > 0),
        });
        await apiFetch(fsURL(projectID, sessionHash, `write?${q}`), {
            method: "PUT",
            headers: { "Content-Type": "application/octet-stream" },
            body: slice,
        });
    }
}

// ---------- Terminal (project-scoped /api/v1/projects/:pid/agents/:id/terminal/ws)
// The v2 browser terminal endpoint (internal/api/handler_terminal_v2.go)
// uses subprotocol "tty" and binary frames shaped [opcode byte][payload...]:
//   '0' = INPUT (c→s) / OUTPUT & STDERR (s→c)
//   '1' = RESIZE {columns,rows}  (c→s; also required as the FIRST frame)
//   '2'/'3' = pause/resume (ignored, legacy zmodem)
//
// The server blocks on the first WS read and uses that resize frame to
// learn cols/rows before opening the agent-side PROCESS_OPEN stream —
// Terminal.tsx sends a ResizeTerminal() call synchronously after
// OpenTerminal resolves, which naturally satisfies that invariant.
//
// Auth: the route is gated by RequireAuthWS + project-RBAC, but
// browsers can't set Authorization on a WebSocket upgrade. We pass the
// JWT as a "Bearer.<jwt>" Sec-WebSocket-Protocol entry alongside "tty";
// the server picks it out for auth and only echoes "tty" back, so the
// auth sentinel is dropped before the live connection starts.

const OP_INPUT = 0x30; // '0'
const OP_RESIZE = 0x31; // '1'

interface Term {
    ws: WebSocket;
    closed: boolean;
}

const terminals = new Map<string, Term>();
let termSeq = 0;

function wsURL(path: string): string {
    const base = getServerURL();
    if (!base) throw new Error("no server URL set");
    return base.replace(/^http/, "ws") + path;
}

export async function OpenTerminal(projectID: string, sessionHash: string): Promise<string> {
    const termID = `t${++termSeq}`;
    const session = getSession();
    if (!session) throw new Error("not connected — log in first");
    const ws = new WebSocket(
        wsURL(
            "/api/v1/projects/" + encodeURIComponent(projectID) +
                "/agents/" + encodeURIComponent(sessionHash) + "/terminal/ws",
        ),
        ["tty", "Bearer." + session.sessionToken],
    );
    ws.binaryType = "arraybuffer";

    const term: Term = { ws, closed: false };
    terminals.set(termID, term);

    ws.addEventListener("message", (ev) => {
        if (!(ev.data instanceof ArrayBuffer)) return;
        const buf = new Uint8Array(ev.data);
        if (buf.length === 0) return;
        const op = buf[0];
        const payload = buf.subarray(1);
        switch (op) {
            case 0x30: // OUTPUT / STDERR → base64-encode to match the desktop
                // Wails contract (Terminal.tsx does atob(b64)).
                emitEvent(`terminal:output:${termID}`, uint8ToBase64(payload));
                break;
            // Other opcodes (title, prefs) aren't emitted by the v2 handler.
        }
    });

    ws.addEventListener("close", () => {
        if (term.closed) return;
        term.closed = true;
        terminals.delete(termID);
        emitEvent(`terminal:closed:${termID}`);
    });

    ws.addEventListener("error", () => {
        // close handler fires next; nothing special to do.
    });

    // Wait for open or error before returning — matches desktop's contract
    // that OpenTerminal's Promise resolves when the session is ready.
    await new Promise<void>((resolve, reject) => {
        ws.addEventListener("open", () => resolve(), { once: true });
        ws.addEventListener("error", () => reject(new Error("terminal ws failed to open")), {
            once: true,
        });
    });

    return termID;
}

export async function SendTerminalInput(termID: string, data: number[]): Promise<void> {
    const t = terminals.get(termID);
    if (!t || t.closed) return;
    const frame = new Uint8Array(1 + data.length);
    frame[0] = OP_INPUT;
    frame.set(data, 1);
    t.ws.send(frame);
}

export async function ResizeTerminal(termID: string, cols: number, rows: number): Promise<void> {
    const t = terminals.get(termID);
    if (!t || t.closed) return;
    const payload = new TextEncoder().encode(JSON.stringify({ Columns: cols, Rows: rows }));
    const frame = new Uint8Array(1 + payload.length);
    frame[0] = OP_RESIZE;
    frame.set(payload, 1);
    t.ws.send(frame);
}

export async function CloseTerminal(termID: string): Promise<void> {
    const t = terminals.get(termID);
    if (!t) return;
    t.closed = true;
    t.ws.close();
    terminals.delete(termID);
}

// Local utility — base64-encode a Uint8Array (Terminal.tsx decodes via atob).
function uint8ToBase64(bytes: Uint8Array): string {
    let binary = "";
    for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
    return btoa(binary);
}
