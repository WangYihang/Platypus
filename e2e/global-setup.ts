import { ChildProcess, spawn } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import * as path from "node:path";
import * as YAML from "yaml";

import {
    ADMIN_PASSWORD,
    ADMIN_USERNAME,
    AGENT_BINARY,
    BACKEND_HOST,
    BACKEND_PORT,
    SCREENSHOT_DIR,
    SEEDED_LISTENER_HOST,
    SEEDED_LISTENER_PORT,
    SEEDED_PROJECTS,
    SERVER_BINARY,
    backendURL,
    makeTmpdir,
} from "./fixtures/env";
import {
    bootstrapAdmin,
    createListener,
    createProject,
    listProjects,
    waitForBackend,
    waitForSessions,
} from "./fixtures/api";

// globalSetup is the once-per-run bootstrapper. It:
//   1. asserts the backend + agent binaries exist (build them via make)
//   2. spawns ./build/platypus-server with a freshly-written temp
//      config that points at a temp SQLite file
//   3. captures the bootstrap secret from stdout
//   4. waits for the REST API to respond
//   5. POSTs /auth/bootstrap to create the admin user
//   6. POSTs /projects to seed two projects + one listener
//   7. spawns one ./build/platypus-agent against the seeded listener
//      and polls the sessions API until the session row appears
//
// Outputs (via process.env) consumed by the rest of the run:
//   PLATYPUS_E2E_TMPDIR              temp dir holding config + db; teardown removes it
//   PLATYPUS_E2E_PID                 backend PID; teardown SIGKILLs
//   PLATYPUS_E2E_AGENT_PID           baseline agent PID; teardown SIGTERMs
//   PLATYPUS_E2E_PROJECTS            JSON of seeded projects { slug, id }
//   PLATYPUS_E2E_BASELINE_SESSION    session_id of the baseline agent
//   PLATYPUS_E2E_BASELINE_HOST       host_id of the baseline agent
//   PLATYPUS_E2E_ADMIN_TOKEN         a fresh JWT access token for spec-side API calls
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
    const config = {
        // Empty listeners — we'll create our own via the REST API once
        // the server is up, on a port that won't collide with config.yml.
        listeners: [],
        restful: {
            host: BACKEND_HOST,
            port: BACKEND_PORT,
            enable: true,
            DBFile: dbPath,
            // Stable JWT keys mean the run is reproducible; otherwise
            // every cold start mints fresh keys and tokens churn.
            JWTAccessKey: "e2e-access-key-do-not-use-in-prod",
            JWTRefreshKey: "e2e-refresh-key-do-not-use-in-prod",
        },
        distributor: {
            host: BACKEND_HOST,
            port: 13340,
            url: `http://${BACKEND_HOST}:13340`,
            store: {
                endpoint: "localhost:9000",
                bucket: "e2e",
            },
        },
        update: false,
        openBrowser: false,
    };
    writeFileSync(configPath, YAML.stringify(config), "utf8");

    // platypus-server reads config.yml from cwd (no --config flag), so
    // spawn with the tmpdir as cwd. Any artefact files the server writes
    // (logs etc) end up in tmpdir too and get cleaned up by teardown.
    const backend = spawn(SERVER_BINARY, [], {
        cwd: tmpdir,
        stdio: ["ignore", "pipe", "pipe"],
        env: { ...process.env },
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

    // After /auth/login starts answering, the secret line should already
    // be in our buffer. Poll briefly to be safe.
    const secretDeadline = Date.now() + 5_000;
    while (!bootstrapSecret && Date.now() < secretDeadline) {
        const m = backendBuf.match(secretRegex);
        if (m) bootstrapSecret = m[1];
        else await new Promise((r) => setTimeout(r, 100));
    }
    if (!bootstrapSecret) {
        throw new Error(
            "could not extract bootstrap secret from backend stdout. " +
                "Set E2E_VERBOSE_BACKEND=1 and re-run to inspect output.",
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

    // One seeded listener in the first project. Bound on a free port
    // (13399) to avoid colliding with config.yml's :13337.
    const defaultProject = seeded[0];
    await createListener(
        backendURL,
        auth.access_token,
        defaultProject.id,
        SEEDED_LISTENER_HOST,
        SEEDED_LISTENER_PORT,
    );

    // ---- Baseline agent --------------------------------------------------
    //
    // Spawn one platypus-agent against the seeded listener so every
    // spec sees populated Hosts/Sessions/Overview state. Agent --token
    // is required by the CLI but unused by Connect (internal/agent/agent.go:52).
    // The connection is TLS+protobuf with InsecureSkipVerify, so no CA
    // setup is needed.
    const agent = spawn(
        AGENT_BINARY,
        ["--host", BACKEND_HOST, "--port", String(SEEDED_LISTENER_PORT), "--token", "e2e"],
        {
            cwd: tmpdir,
            stdio: ["ignore", "pipe", "pipe"],
            env: { ...process.env, HOME: tmpdir },
        },
    );
    process.env.PLATYPUS_E2E_AGENT_PID = String(agent.pid);

    const agentReaper = () => {
        try {
            if (agent.pid) process.kill(agent.pid, "SIGKILL");
        } catch {
            /* already gone */
        }
    };
    process.once("exit", agentReaper);
    process.once("SIGINT", agentReaper);
    process.once("SIGTERM", agentReaper);

    if (process.env.E2E_VERBOSE_AGENT) {
        agent.stdout?.on("data", (c: Buffer) => process.stdout.write(`[agent] ${c}`));
        agent.stderr?.on("data", (c: Buffer) => process.stderr.write(`[agent!] ${c}`));
    }

    // Reuse the bootstrap's access_token for spec-side API calls.
    // Calling /auth/login back-to-back can race the refresh-token
    // persist (500 "persist refresh") under the same DB.
    process.env.PLATYPUS_E2E_ADMIN_TOKEN = auth.access_token;

    const live = await waitForSessions(
        backendURL,
        auth.access_token,
        defaultProject.id,
        1,
        15_000,
    );
    process.env.PLATYPUS_E2E_BASELINE_SESSION = live[0].id;
    process.env.PLATYPUS_E2E_BASELINE_HOST = live[0].host_id;
}

// Re-export the backend handle for teardown via the env-passed PID.
export type _BackendHandle = ChildProcess;
