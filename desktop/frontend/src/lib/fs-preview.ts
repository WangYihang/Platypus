// Web-mode helper: mint a short-lived signed URL for /fs/read so a
// browser-native <video src=...> / <audio src=...> / pdf.js fetch can
// authenticate without an Authorization header. Lives in lib/ rather
// than under platform/App.web.ts because it's web-mode-only — the
// Wails desktop binding has no FsReadPreviewURL method, so exporting
// it from the @wails/* alias would produce a runtime undefined in
// desktop mode. Viewer components that want to work in both modes
// branch on import.meta.env.MODE before calling this.
//
// The token is bound to (project, agent, path) and expires after 5
// minutes (server-side default in PreviewSigner). Mint immediately
// before opening the viewer so the TTL covers the user's first
// interaction; cached URLs go stale fast and that's by design.

import { authJSON, getSession } from "./auth";

interface MintResponse {
    token: string;
    exp: number;
    url: string; // server-relative /api/v1/projects/.../fs/read?…
}

// fsReadPreviewURL POSTs to /fs/preview-token (Bearer-auth gated, so
// anonymous calls get 401) and returns the absolute URL the caller
// can drop into a media element's src. Absolute because the page is
// usually served from a different origin than the API in web mode.
export async function fsReadPreviewURL(
    projectID: string,
    sessionHash: string,
    path: string,
): Promise<string> {
    const endpoint =
        "/api/v1/projects/" + encodeURIComponent(projectID) +
        "/agents/" + encodeURIComponent(sessionHash) +
        "/fs/preview-token";
    const minted = await authJSON<MintResponse>(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path }),
    });
    const session = getSession();
    if (!session) throw new Error("not connected — log in first");
    return session.serverURL + minted.url;
}
