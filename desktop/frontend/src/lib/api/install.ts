import { authFetch, authJSON } from "../auth";

// "Generate a one-shot curl command to install an agent". The returned
// install_command is ready to paste.

export interface InstallArtifactListItem {
    download_id: string;
    project_id: string;
    issued_by_user: string;
    issued_at: string;
    expires_at: string;
    server_endpoint: string;
    target_os?: string;
    target_arch?: string;
    pat_ttl_seconds: number;
    pat_binding_machine_id?: string;
    pat_description?: string;
    consumed_at?: string;
    consumed_ip?: string;
    consumed_pat_id?: string;
    auto_approve?: boolean;
    revoked: boolean;
    revoked_at?: string;
    status: "pending" | "consumed" | "expired" | "revoked";
}

export interface IssueInstallResponse {
    download_id: string;
    download_token: string; // dl_<id>.<secret>
    expires_at: string;
    server_endpoint: string;
    target_os?: string;
    target_arch?: string;
    // OS-default downloader's one-liner (curl on unix, powershell on
    // windows). Older callers / tests that don't know about the
    // per-downloader map still get something sensible here.
    install_command: string;
    // Per-downloader one-liners keyed by downloader name in the
    // "skip TLS verification" flavour (`-k`, --no-check-certificate,
    // ServerCertificateValidationCallback={$true}, etc). Default
    // when the wizard's "Skip TLS verification" toggle is ON.
    // Optional for backwards-compat with old server builds.
    install_commands?: Record<string, string>;
    // Same per-downloader map but WITHOUT skip-cert flags — for
    // prod deployments where the install endpoint serves a
    // system-trusted cert and the operator wants a clean,
    // MITM-resistant one-liner. Surfaced via the wizard's "Skip
    // TLS verification" toggle when OFF.
    install_commands_strict?: Record<string, string>;
    // Per-downloader bundle one-liners — `platypus-agent "$(...)"`
    // (unix) or PowerShell equivalent. Same single-use install
    // token under the hood (consuming script burns it equally).
    // The FE used to compose these client-side; now they come from
    // the BE registry so insecure-flag conventions live in one
    // place.
    bundle_commands?: Record<string, string>;
    bundle_commands_strict?: Record<string, string>;
    // pinst_<base64> self-contained token (when curled). For targets
    // that can't pipe to a shell — paste the resulting token straight
    // into platypus-agent. Same single-use install token as install_command.
    bundle_url: string;
}

export interface IssueInstallRequest {
    server_endpoint: string;
    target_os?: string;
    target_arch?: string;
    ttl_seconds?: number;
    pat_ttl_seconds?: number;
    pat_binding_machine_id?: string;
    pat_description?: string;
    // false (default) → host enrolls in `pending`. true skips approval
    // for unattended automation (Ansible / CI / cloud-init).
    auto_approve?: boolean;
}

export async function listInstallArtifacts(
    pid: string,
    includeInactive = false,
): Promise<InstallArtifactListItem[]> {
    const q = includeInactive ? "?include_inactive=true" : "";
    const j = await authJSON<{ install_artifacts: InstallArtifactListItem[] }>(
        `/api/v1/projects/${pid}/install-artifacts${q}`,
    );
    return j.install_artifacts ?? [];
}

export async function issueInstallArtifact(
    pid: string,
    req: IssueInstallRequest,
): Promise<IssueInstallResponse> {
    return authJSON<IssueInstallResponse>(
        `/api/v1/projects/${pid}/install-artifacts`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(req),
        },
    );
}

export async function revokeInstallArtifact(
    pid: string,
    downloadID: string,
    reason?: string,
): Promise<void> {
    const q = reason ? `?reason=${encodeURIComponent(reason)}` : "";
    const r = await authFetch(
        `/api/v1/projects/${pid}/install-artifacts/${downloadID}${q}`,
        { method: "DELETE" },
    );
    if (!r.ok && r.status !== 404) throw new Error(`${r.status}: ${await r.text()}`);
}

// One (os, arch) pair the active channel's manifest pins. The Issue
// Install dialog uses this to populate its picker so admins can only
// choose targets the distributor can actually serve.
export interface InstallPlatform {
    os: string;
    arch: string;
}

export interface InstallPlatformsResponse {
    channel: string;
    // Empty when no manifest published yet — response stays 200 so the
    // dialog can render a clear "publish first" hint.
    version: string;
    platforms: InstallPlatform[];
}

export async function listInstallPlatforms(): Promise<InstallPlatformsResponse> {
    return authJSON<InstallPlatformsResponse>("/api/v1/install/platforms");
}
