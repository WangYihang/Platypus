import { mkdtempSync } from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

// Centralised env constants used by globalSetup, globalTeardown, the
// Playwright config, and the spec files. Single source of truth so
// changing a port or path doesn't ripple through six files.

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// __dirname here resolves to <repo>/e2e/fixtures/, so two levels up is repo root.
export const REPO_ROOT = path.resolve(__dirname, "..", "..");
export const FRONTEND_DIR = path.resolve(REPO_ROOT, "desktop", "frontend");
export const E2E_DIR = path.resolve(REPO_ROOT, "e2e");
export const SCREENSHOT_DIR = path.resolve(REPO_ROOT, "docs", "screenshots");
export const SERVER_BINARY = path.resolve(REPO_ROOT, "build", "platypus-server");
export const AGENT_BINARY = path.resolve(REPO_ROOT, "build", "platypus-agent");

export const BACKEND_HOST = "127.0.0.1";
export const BACKEND_PORT = 7332;
export const FRONTEND_HOST = "127.0.0.1";
export const FRONTEND_PORT = 5173;

// Listener bound by the seed step. Picked away from config.yml's
// :13337 default so a manually-running dev server doesn't collide.
export const SEEDED_LISTENER_HOST = "127.0.0.1";
export const SEEDED_LISTENER_PORT = 13399;

// Seeded admin credentials. Specs log in via the UI with these.
export const ADMIN_USERNAME = "admin";
export const ADMIN_PASSWORD = "admin12345";

export const MESH_PSK = "e2e-mesh-psk-must-be-32-chars-long!!";
export const MESH_PSK_FILENAME = "mesh.psk";

// Two seeded projects.
export const SEEDED_PROJECTS = [
    { slug: "default", name: "Default" },
    { slug: "staging", name: "Staging" },
] as const;

export const baseURL = `http://${FRONTEND_HOST}:${FRONTEND_PORT}`;
export const backendURL = `http://${BACKEND_HOST}:${BACKEND_PORT}`;

// makeTmpdir creates a unique temp directory used by the test run for
// the temp config + SQLite + backend logs. Returned path is set into
// process.env.PLATYPUS_E2E_TMPDIR so globalTeardown can clean up.
export function makeTmpdir(): string {
    return mkdtempSync(path.join(os.tmpdir(), "platypus-e2e-"));
}
