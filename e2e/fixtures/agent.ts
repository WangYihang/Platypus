import { ChildProcess, spawn } from "node:child_process";
import { Buffer } from "node:buffer";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { test as base } from "@playwright/test";

import { AGENT_BINARY, BACKEND_HOST, BACKEND_PORT, MESH_PSK_FILENAME, backendURL } from "./env";
import { getProjectCA, issuePAT, listProjectHosts, waitForHosts } from "./api";

// Per-project enrollment credentials minted lazily on first use and
// shared across every spec in the run. max_uses is bumped high enough
// that dozens of agents can redeem the single PAT. The CA comes from
// /api/v1/projects/:pid/ca — since the server auto-issues its ingress
// leaf from the same project CA (see cmd/platypus-server/main.go ::
// issueIngressLeafFromProjectCA), pinning that CA in the agent
// completes the handshake without any separate certificate wiring.
interface EnrollCreds {
    pat: string;
    caBase64: string;
}
const enrollCache = new Map<string, Promise<EnrollCreds>>();

function getAdminToken(): string {
    const tok = process.env.PLATYPUS_E2E_ADMIN_TOKEN;
    if (!tok) throw new Error("PLATYPUS_E2E_ADMIN_TOKEN not set (globalSetup failed?)");
    return tok;
}

async function ensureEnrollCreds(projectID: string): Promise<EnrollCreds> {
    const cached = enrollCache.get(projectID);
    if (cached) return cached;
    const p = (async () => {
        const adminToken = getAdminToken();
        // max_uses = 100 covers every agent the test suite spawns —
        // MintPAT coerces 0 to 1 (single-use) which would starve the
        // second spec that asks for an agent.
        const pat = await issuePAT(backendURL, adminToken, projectID, {
            description: "e2e-agent-enrollment",
            ttl_seconds: 3600,
            max_uses: 100,
        });
        const ca = await getProjectCA(backendURL, adminToken, projectID);
        return {
            pat: pat.token,
            caBase64: Buffer.from(ca.cert_pem, "utf8").toString("base64"),
        };
    })();
    enrollCache.set(projectID, p);
    return p;
}

export interface AgentHandle {
    pid: number;
    proc: ChildProcess;
    kill: () => Promise<void>;
}

// spawnAgent is the canonical way to start a platypus-agent against
// the e2e backend. Handles PAT minting, project CA injection, identity
// dir isolation, and the host-appears wait.
//
// Mesh is opt-in: set pskFile (and optionally meshListen / peers).
interface SpawnAgentOpts {
    projectID: string;
    labelForLogs?: string;
    meshListen?: string;
    peers?: string[];
    pskFile?: string;
    waitForHost?: boolean; // default true; waits until a new host row appears
    waitTimeoutMs?: number; // default 15_000
}

export async function spawnAgent(opts: SpawnAgentOpts): Promise<AgentHandle> {
    const creds = await ensureEnrollCreds(opts.projectID);
    const adminToken = getAdminToken();
    const beforeCount = (
        await listProjectHosts(backendURL, adminToken, opts.projectID)
    ).length;

    const agentHome = fs.mkdtempSync(path.join(os.tmpdir(), "platypus-e2e-agent-"));

    const args = [
        "--host",
        BACKEND_HOST,
        "--port",
        String(BACKEND_PORT),
        "--token",
        creds.pat,
        "--identity-dir",
        agentHome,
    ];
    if (opts.pskFile) args.push("--psk-file", opts.pskFile);
    if (opts.meshListen) args.push("--mesh-listen", opts.meshListen);
    for (const p of opts.peers || []) args.push("--peers", p);

    const proc = spawn(AGENT_BINARY, args, {
        stdio: ["ignore", "pipe", "pipe"],
        env: {
            ...process.env,
            PLATYPUS_PROJECT_CA: creds.caBase64,
        },
    });
    if (!proc.pid) throw new Error(`failed to spawn agent: ${opts.labelForLogs || "agent"}`);

    const label = opts.labelForLogs || "agent";
    if (process.env.E2E_VERBOSE_AGENT) {
        proc.stdout?.on("data", (c: Buffer) => process.stdout.write(`[${label}] ${c}`));
        proc.stderr?.on("data", (c: Buffer) => process.stderr.write(`[${label}!] ${c}`));
    }

    if (opts.waitForHost !== false) {
        await waitForHosts(
            backendURL,
            adminToken,
            opts.projectID,
            beforeCount + 1,
            opts.waitTimeoutMs ?? 15_000,
        );
    }

    return {
        pid: proc.pid,
        proc,
        async kill() {
            try {
                process.kill(proc.pid!, "SIGTERM");
            } catch {
                /* already gone */
            }
            await new Promise<void>((resolve) => {
                if (proc.exitCode !== null || proc.signalCode) return resolve();
                const t = setTimeout(() => {
                    try {
                        process.kill(proc.pid!, "SIGKILL");
                    } catch {
                        /* already gone */
                    }
                    resolve();
                }, 2_000);
                proc.once("exit", () => {
                    clearTimeout(t);
                    resolve();
                });
            });
            try {
                fs.rmSync(agentHome, { recursive: true, force: true });
            } catch {
                /* best-effort */
            }
        },
    };
}

// startExtraAgent is the signature multi-agent specs call: spawn one
// extra agent against the project, wait until a new session appears.
export function startExtraAgent(projectID: string, _adminToken?: string): Promise<AgentHandle> {
    return spawnAgent({ projectID, labelForLogs: "agent+" });
}

// startMeshAgent spawns an agent with mesh networking enabled.
export function startMeshAgent(
    projectID: string,
    _adminToken: string,
    opts: { meshListen: string; peers?: string[] },
): Promise<AgentHandle> {
    const tmpdir = process.env.PLATYPUS_E2E_TMPDIR!;
    return spawnAgent({
        projectID,
        labelForLogs: "mesh-agent",
        pskFile: path.join(tmpdir, MESH_PSK_FILENAME),
        meshListen: opts.meshListen,
        peers: opts.peers,
    });
}

// startZeroConfigAgent spawns an agent without any mesh peer list so
// it has to discover peers on its own (mDNS / server-pushed bootstrap).
export function startZeroConfigAgent(
    projectID: string,
    _adminToken: string,
): Promise<AgentHandle> {
    return spawnAgent({ projectID, labelForLogs: "zero-agent" });
}

// liveAgent fixture: spawns one agent for the "default" project and
// tears it down at the end of the spec. Specs that only need "any
// agent is online" import { expect, test } from "../fixtures/agent"
// and declare liveAgent as a fixture arg.
export const test = base.extend<{ liveAgent: AgentHandle }>({
    liveAgent: async ({}, use) => {
        const projects = JSON.parse(
            process.env.PLATYPUS_E2E_PROJECTS || "[]",
        ) as Array<{ slug: string; id: string }>;
        const def = projects.find((p) => p.slug === "default");
        if (!def) throw new Error("default project missing from PLATYPUS_E2E_PROJECTS");
        const handle = await spawnAgent({ projectID: def.id, labelForLogs: "live-agent" });
        await use(handle);
        await handle.kill();
    },
});

export { expect } from "@playwright/test";
