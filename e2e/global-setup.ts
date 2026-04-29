import { ChildProcess, spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import * as path from "node:path";
import * as YAML from "yaml";

// Backend ingress serves a self-signed TLS cert on dev / first-boot.
// Node's built-in fetch refuses those by default; since this is the
// e2e harness we explicitly opt out of verification for the whole
// process. The browser-side equivalent lives in playwright.config.ts
// (`use: { ignoreHTTPSErrors: true }`).
process.env.NODE_TLS_REJECT_UNAUTHORIZED = "0";

import {
    ADMIN_PASSWORD,
    ADMIN_USERNAME,
    AGENT_BINARY,
    BACKEND_HOST,
    BACKEND_PORT,
    MESH_PSK,
    MESH_PSK_FILENAME,
    SCREENSHOT_DIR,
    SEEDED_PROJECTS,
    SERVER_BINARY,
    backendURL,
    makeTmpdir,
} from "./fixtures/env";
import { bootstrapAdmin, createProject, listProjects, waitForBackend } from "./fixtures/api";

// globalSetup is the once-per-run bootstrapper. It:
//   1. asserts the backend + agent binaries exist (build them via make)
//   2. spawns ./build/platypus-server with a freshly-written temp
//      config that points at a temp SQLite file
//   3. captures the bootstrap secret from stdout
//   4. waits for the REST API to respond
//   5. POSTs /auth/bootstrap to create the admin user
//   6. POSTs /projects to seed the "staging" project (the "default"
//      project is auto-seeded by the server on first boot)
//
// Outputs (via process.env) consumed by the rest of the run:
//   PLATYPUS_E2E_TMPDIR   temp dir holding config + db; teardown removes it
//   PLATYPUS_E2E_PID      backend PID; teardown SIGKILLs
//   PLATYPUS_E2E_PROJECTS JSON of seeded projects { slug, id }
//   PLATYPUS_E2E_ADMIN_TOKEN a fresh JWT access token for spec-side API calls
//
// Baseline agent + per-session-fixtures are deliberately not started
// here — they now require PAT-based enrollment against the project
// CA, which is a larger rewrite landing in a follow-up commit. Specs
// that need live sessions spawn their own agents via fixtures/agent.ts.
//
// Frontend lifecycle is Playwright's webServer (see playwright.config.ts)
// — no double-managing.
export default async function globalSetup() {
    if (!existsSync(SERVER_BINARY)) {
        throw new Error(
            [
                `Backend binary not found at ${SERVER_BINARY}.`,
                "Build it first:",
                "    cd <repo-root> && make build",
                "Then re-run the e2e suite.",
            ].join("\n"),
        );
    }
    if (!existsSync(AGENT_BINARY)) {
        throw new Error(
            [
                `Agent binary not found at ${AGENT_BINARY}.`,
                "Build it first:",
                "    cd <repo-root> && make build",
                "Then re-run the e2e suite.",
            ].join("\n"),
        );
    }

    if (!existsSync(SCREENSHOT_DIR)) {
        mkdirSync(SCREENSHOT_DIR, { recursive: true });
    }

    const tmpdir = makeTmpdir();
    process.env.PLATYPUS_E2E_TMPDIR = tmpdir;

    const dbPath = path.join(tmpdir, "platypus.db");
    const configPath = path.join(tmpdir, "config.yml");

    // Matches internal/utils/config/config.go (snake_case yaml tags).
    // Listeners / distributor / mesh listen_addr all moved onto the
    // unified ingress; distributor.store endpoint left empty so the
    // installer routes are disabled (no MinIO dependency in e2e).
    // mesh.project_id is "default" to align with the system-seeded
    // project row (storage.DefaultProjectID) — using any other value
    // would fail project_ca's FK on startup.
    //
    // Leaving ingress.cert/key unset triggers the server's new
    // auto-issue path (cmd/platypus-server/main.go :: issueIngressLeaf
    // FromProjectCA), which stamps a TLS leaf signed by the project
    // CA. Agents that pin PLATYPUS_PROJECT_CA (fetched via
    // /api/v1/projects/:pid/ca after bootstrap) verify that chain.
    const config = {
        ingress: {
            addr: `${BACKEND_HOST}:${BACKEND_PORT}`,
            public_addr: `${BACKEND_HOST}:${BACKEND_PORT}`,
        },
        restful: {
            db_file: dbPath,
            // Stable JWT keys mean the run is reproducible; otherwise
            // every cold start mints fresh keys and tokens churn.
            jwt_access_key: "e2e-access-key-do-not-use-in-prod",
            jwt_refresh_key: "e2e-refresh-key-do-not-use-in-prod",
        },
        mesh: {
            psk_file: path.join(tmpdir, MESH_PSK_FILENAME),
            discovery_lan: true,
            project_id: "default",
        },
    };
    writeFileSync(configPath, YAML.stringify(config), "utf8");

    // Write the Mesh PSK file for use by agents.
    writeFileSync(path.join(tmpdir, MESH_PSK_FILENAME), MESH_PSK, "utf8");

    // platypus-server reads config.yml from cwd (no --config flag), so
    // spawn with the tmpdir as cwd. Any artefact files the server writes
    // (logs etc) end up in tmpdir too and get cleaned up by teardown.
    //
    // PLATYPUS_DEV=1 opts the e2e backend into the dev-only on-disk KEK
    // fallback at <data-dir>/ca.kek. Without it the server refuses to
    // start when PLATYPUS_CA_KEK is unset (which it is in e2e).
    const backend = spawn(SERVER_BINARY, [], {
        cwd: tmpdir,
        stdio: ["ignore", "pipe", "pipe"],
        env: { ...process.env, PLATYPUS_DEV: "1" },
    });
    process.env.PLATYPUS_E2E_PID = String(backend.pid);

    // Safety net: if globalSetup or any spec throws and Node exits before
    // globalTeardown runs, still SIGKILL the backend. Stdin is "ignore"
    // so the interactive "Exit? Y/N" prompt would loop forever otherwise.
    const reaper = () => {
        try {
            if (backend.pid) process.kill(backend.pid, "SIGKILL");
        } catch {
            /* already gone */
        }
    };
    process.once("exit", reaper);
    process.once("SIGINT", reaper);
    process.once("SIGTERM", reaper);

    let backendBuf = "";
    let bootstrapSecret = "";
    const secretRegex = /(?:API bootstrap secret:|["']bootstrap_secret["']:\s*["'])([0-9a-f]+)/i;

    // The Go server writes the bootstrap secret via the platypus log
    // package which routes through os.Stderr (with a timestamp prefix);
    // other lines also come on stderr. Scan both streams just in case.
    const onChunk = (s: string) => {
        backendBuf += s;
        if (!bootstrapSecret) {
            const m = backendBuf.match(secretRegex);
            if (m) bootstrapSecret = m[1];
        }
    };
    backend.stdout?.on("data", (chunk: Buffer) => {
        const s = chunk.toString();
        onChunk(s);
        if (process.env.E2E_VERBOSE_BACKEND) process.stdout.write(`[backend] ${s}`);
    });
    backend.stderr?.on("data", (chunk: Buffer) => {
        const s = chunk.toString();
        onChunk(s);
        if (process.env.E2E_VERBOSE_BACKEND) process.stderr.write(`[backend!] ${s}`);
    });
    backend.once("exit", (code) => {
        if (!backend.killed) {
            console.error(`[e2e] platypus-server exited unexpectedly (code=${code})`);
            console.error(backendBuf);
        }
    });

    // Wait for the backend to print the secret OR to start serving.
    // The secret line lands almost immediately at startup; serving
    // takes a beat longer. Wait for both.
    await waitForBackend(backendURL, 30_000);

    // After /auth/login starts answering, the secret should be readable.
    // Preferred path: <data-dir>/bootstrap.secret, written 0600 on first
    // boot when the users table is empty (cmd/platypus-server/main.go).
    // Fallback: scan stdout for a legacy "bootstrap_secret":"<hex>" line
    // in case we're running against an older binary.
    const bootstrapSecretFile = path.join(tmpdir, "bootstrap.secret");
    const secretDeadline = Date.now() + 5_000;
    while (!bootstrapSecret && Date.now() < secretDeadline) {
        if (existsSync(bootstrapSecretFile)) {
            bootstrapSecret = readFileSync(bootstrapSecretFile, "utf8").trim();
            break;
        }
        const m = backendBuf.match(secretRegex);
        if (m) {
            bootstrapSecret = m[1];
            break;
        }
        await new Promise((r) => setTimeout(r, 100));
    }
    if (!bootstrapSecret) {
        throw new Error(
            `could not read bootstrap secret from ${bootstrapSecretFile} ` +
                "or backend stdout. Set E2E_VERBOSE_BACKEND=1 and re-run to inspect output.",
        );
    }

    // Seed: bootstrap admin → login → create projects + a listener.
    const auth = await bootstrapAdmin(backendURL, {
        secret: bootstrapSecret,
        username: ADMIN_USERNAME,
        password: ADMIN_PASSWORD,
    });

    // Bootstrap auto-seeds a "Default" project (handler_auth_v1.go),
    // so we look that one up by slug and only create the extras.
    const existing = await listProjects(backendURL, auth.access_token);
    const seeded: Array<{ slug: string; id: string }> = [];
    for (const p of SEEDED_PROJECTS) {
        const found = existing.find((x) => x.slug === p.slug);
        if (found) {
            seeded.push({ slug: found.slug, id: found.id });
        } else {
            const created = await createProject(
                backendURL,
                auth.access_token,
                p.name,
                p.slug,
            );
            seeded.push({ slug: created.slug, id: created.id });
        }
    }
    process.env.PLATYPUS_E2E_PROJECTS = JSON.stringify(seeded);

    // Reuse the bootstrap's access_token for spec-side API calls.
    // Calling /auth/login back-to-back can race the refresh-token
    // persist (500 "persist refresh") under the same DB.
    process.env.PLATYPUS_E2E_ADMIN_TOKEN = auth.access_token;

    // Spawn one baseline agent for the "default" project. Specs that
    // need "an agent is online" assert on host rows directly rather
    // than spawning their own per-test agent; multiple agents on the
    // same machine would collide on the hosts.(project_id, fingerprint)
    // UNIQUE constraint (every agent binary picks up the same
    // os.Hostname() / machine-id, so fingerprints match). Single
    // shared fixture sidesteps that without needing a machine-id CLI
    // flag on the agent.
    const defaultProject = seeded.find((p) => p.slug === "default");
    if (!defaultProject) {
        throw new Error("globalSetup: default project missing after seeding");
    }
    await startBaselineAgent(tmpdir, defaultProject.id, auth.access_token);
}

async function startBaselineAgent(
    tmpdir: string,
    projectID: string,
    adminToken: string,
): Promise<void> {
    const patResp = (await (
        await fetch(`${backendURL}/api/v1/projects/${projectID}/pat-tokens`, {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                Authorization: `Bearer ${adminToken}`,
            },
            body: JSON.stringify({
                description: "e2e-baseline",
                ttl_seconds: 3600,
                max_uses: 100,
            }),
        })
    ).json()) as { token: string };
    const caResp = (await (
        await fetch(`${backendURL}/api/v1/projects/${projectID}/ca`, {
            headers: { Authorization: `Bearer ${adminToken}` },
        })
    ).json()) as { cert_pem: string };
    const caBase64 = Buffer.from(caResp.cert_pem, "utf8").toString("base64");

    const agentHome = path.join(tmpdir, "baseline-agent");
    mkdirSync(agentHome, { recursive: true });

    // Agent CLI in 93fdd65 onwards: install-token is a positional
    // argument (not --token), and the persistent-state directory
    // moved from --identity-dir to --data-dir. Server host/port can
    // still be forced with --host / --port (used here so the e2e
    // backend's loopback bind works regardless of what's embedded
    // in the freshly-minted PAT).
    const agent = spawn(
        AGENT_BINARY,
        [
            "--server",
            `${BACKEND_HOST}:${BACKEND_PORT}`,
            "--data-dir",
            agentHome,
            patResp.token,
        ],
        {
            cwd: tmpdir,
            stdio: ["ignore", "pipe", "pipe"],
            env: { ...process.env, PLATYPUS_PROJECT_CA: caBase64 },
        },
    );
    process.env.PLATYPUS_E2E_AGENT_PID = String(agent.pid);

    const reaper = () => {
        try {
            if (agent.pid) process.kill(agent.pid, "SIGKILL");
        } catch {
            /* already gone */
        }
    };
    process.once("exit", reaper);
    process.once("SIGINT", reaper);
    process.once("SIGTERM", reaper);

    if (process.env.E2E_VERBOSE_AGENT) {
        agent.stdout?.on("data", (c: Buffer) => process.stdout.write(`[baseline-agent] ${c}`));
        agent.stderr?.on("data", (c: Buffer) => process.stderr.write(`[baseline-agent!] ${c}`));
    }

    // Block globalSetup until the host row shows up, so the first
    // spec already has its populated hosts view.
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
        const r = await fetch(`${backendURL}/api/v1/projects/${projectID}/hosts`, {
            headers: { Authorization: `Bearer ${adminToken}` },
        });
        if (r.ok) {
            const { hosts } = (await r.json()) as { hosts?: unknown[] };
            if (hosts && hosts.length > 0) return;
        }
        await new Promise((res) => setTimeout(res, 250));
    }
    throw new Error("globalSetup: baseline agent did not register a host in 15s");
}

// Re-export the backend handle for teardown via the env-passed PID.
export type _BackendHandle = ChildProcess;
