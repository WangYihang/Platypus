// Web-mode drop-in for wailsjs/go/app/App. Every name the pages import
// from that path is re-exported here with a REST/WebSocket implementation
// that talks to platypus-server directly. The server's CORS config is `*`
// (see internal/api/rest.go) so cross-origin fetch + WebSocket work.
//
// Auth comes from lib/auth — the JWT session set up by Login.tsx. The
// in-memory access token there is the source of truth; this shim does
// NOT manage its own token store any more (it used to, back when the
// server only spoke the legacy single-secret /auth/token flow).

import type { api } from "../../wailsjs/go/models";
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

// ---------- Sessions ----------------------------------------------------

export async function ListSessions(): Promise<api.Session[]> {
    const resp = await apiJSON<{ sessions: any[] }>("/api/v1/sessions");
    const out: api.Session[] = (resp.sessions || []).map((raw) => raw as api.Session);
    out.sort((a, b) => (a.hash < b.hash ? -1 : 1));
    return out;
}

export async function SetGroupDispatch(hash: string, enabled: boolean): Promise<void> {
    await apiFetch("/api/v1/sessions/" + encodeURIComponent(hash), {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group_dispatch: enabled }),
    });
}

export async function DispatchCommand(
    command: string,
    timeoutSec: number,
): Promise<api.DispatchResult[]> {
    const resp = await apiJSON<{ results?: api.DispatchResult[] }>("/api/v1/sessions/dispatch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command, timeout: timeoutSec }),
    });
    return resp.results || [];
}

// ---------- Listeners ---------------------------------------------------

export async function ListListeners(): Promise<api.Listener[]> {
    const resp = await apiJSON<{ listeners: any[] }>("/api/v1/listeners");
    return (resp.listeners || []).map((s: any) => {
        const l = { ...s } as api.Listener & { NumSessions: number };
        l.NumSessions = s.agent_clients ? Object.keys(s.agent_clients).length : 0;
        return l;
    });
}

export async function CreateListener(host: string, port: number): Promise<void> {
    await apiFetch("/api/v1/listeners", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host, port }),
    });
}

export async function DeleteListener(hash: string): Promise<void> {
    await apiFetch("/api/v1/listeners/" + encodeURIComponent(hash), { method: "DELETE" });
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

export async function FileSize(sessionHash: string, path: string): Promise<number> {
    const q = new URLSearchParams({ path });
    const resp = await apiJSON<{ status: boolean; size?: number; error?: string }>(
        `/api/v1/sessions/${encodeURIComponent(sessionHash)}/files/size?${q}`,
    );
    if (!resp.status) throw new Error(resp.error || "size failed");
    return resp.size || 0;
}

const CHUNK_SIZE = 256 * 1024;

export async function ReadFile(
    sessionHash: string,
    path: string,
    offset: number,
    size: number,
): Promise<number[]> {
    const q = new URLSearchParams({
        path,
        offset: String(offset),
        size: String(size),
    });
    const r = await apiFetch(`/api/v1/sessions/${encodeURIComponent(sessionHash)}/files?${q}`);
    const buf = new Uint8Array(await r.arrayBuffer());
    return Array.from(buf);
}

export async function WriteFile(
    sessionHash: string,
    path: string,
    data: number[],
    appendMode: boolean,
): Promise<void> {
    const q = new URLSearchParams({ path, append: String(appendMode) });
    await apiFetch(`/api/v1/sessions/${encodeURIComponent(sessionHash)}/files?${q}`, {
        method: "POST",
        headers: { "Content-Type": "application/octet-stream" },
        body: new Uint8Array(data),
    });
}

export async function DownloadFile(
    sessionHash: string,
    remotePath: string,
    localPath: string,
): Promise<void> {
    const name = pendingDownloadNames.get(localPath) || "download.bin";
    pendingDownloadNames.delete(localPath);

    const total = await FileSize(sessionHash, remotePath);
    if (total === 0) throw new Error("remote file is empty or unreadable");

    // BlobPart[] (not Uint8Array[]) so TS 6+ accepts SharedArrayBuffer-
    // backed byte arrays when passing to new Blob(...).
    const parts: BlobPart[] = [];
    for (let off = 0; off < total; off += CHUNK_SIZE) {
        const want = Math.min(CHUNK_SIZE, total - off);
        const q = new URLSearchParams({
            path: remotePath,
            offset: String(off),
            size: String(want),
        });
        const r = await apiFetch(
            `/api/v1/sessions/${encodeURIComponent(sessionHash)}/files?${q}`,
        );
        parts.push(new Uint8Array(await r.arrayBuffer()));
    }
    const blob = new Blob(parts, { type: "application/octet-stream" });
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

export async function UploadFile(
    sessionHash: string,
    remotePath: string,
    localPath: string,
): Promise<void> {
    const f = pendingUploads.get(localPath);
    if (!f) throw new Error(`no pending upload for ${localPath} — did PickFileToUpload run?`);
    pendingUploads.delete(localPath);

    const bytes = new Uint8Array(await f.arrayBuffer());
    // First chunk truncates, rest append — matches Go WriteFile semantics.
    for (let off = 0; off < bytes.length; off += CHUNK_SIZE) {
        const slice = bytes.subarray(off, Math.min(off + CHUNK_SIZE, bytes.length));
        const q = new URLSearchParams({
            path: remotePath,
            append: String(off > 0),
        });
        await apiFetch(`/api/v1/sessions/${encodeURIComponent(sessionHash)}/files?${q}`, {
            method: "POST",
            headers: { "Content-Type": "application/octet-stream" },
            body: slice,
        });
    }
}

// ---------- Tunnels -----------------------------------------------------

export async function ListTunnels(sessionHash: string): Promise<api.TunnelInfo[]> {
    const resp = await apiJSON<{ tunnels?: api.TunnelInfo[] }>(
        `/api/v1/sessions/${encodeURIComponent(sessionHash)}/tunnels`,
    );
    return resp.tunnels || [];
}

export async function CreateTunnel(
    sessionHash: string,
    mode: string,
    srcAddress: string,
    dstAddress: string,
): Promise<void> {
    await apiFetch(`/api/v1/sessions/${encodeURIComponent(sessionHash)}/tunnels`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ mode, src_address: srcAddress, dst_address: dstAddress }),
    });
}

// ---------- Terminal (ported opcode logic from ws_terminal.go) ----------
// The /ws/:hash WebSocket uses subprotocol "tty" and binary frames shaped
// [opcode byte][payload...]:
//   '0' = INPUT (c→s) / OUTPUT (s→c)
//   '1' = RESIZE_TERMINAL {cols,rows} (c→s) / SET_WINDOW_TITLE (s→c)
//   '2' = SET_PREFERENCES (s→c)

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

export async function OpenTerminal(sessionHash: string): Promise<string> {
    const termID = `t${++termSeq}`;
    // Browsers can't set Bearer headers on a WS upgrade, so trade the
    // token for a one-shot ticket first.
    const { ticket } = await apiJSON<{ ticket: string }>("/api/v1/ws/ticket", { method: "POST" });
    const ws = new WebSocket(
        wsURL("/ws/" + encodeURIComponent(sessionHash)) + "?ticket=" + encodeURIComponent(ticket),
        ["tty"],
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
            case 0x30: // OUTPUT → base64-encode to match the desktop Wails contract
                // (Terminal.tsx does atob(b64)).
                emitEvent(`terminal:output:${termID}`, uint8ToBase64(payload));
                break;
            // 0x31 SET_WINDOW_TITLE and 0x32 SET_PREFERENCES are currently ignored
            // (desktop side also doesn't surface them to the page).
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
