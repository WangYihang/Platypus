import { expect, test } from "@playwright/test";

import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../../fixtures/env";
import { caption, clearOverlays, pause } from "../../fixtures/demo";

// 06-auth-redesign — narrated end-to-end of the Phase 2 auth surface:
// a single opaque session_token (`pst_<id>.<secret>`) replaces the
// JWT access / refresh pair. Walks through:
//   1. Login UI → server returns one session_token; the frontend
//      caches it in localStorage under `platypus.sessions`
//   2. Use that bearer against a protected endpoint (/projects)
//   3. Logout invalidates the bearer server-side; the same bearer
//      stops working immediately (no 30s cache lag)
//
// (Earlier revisions of this demo also showcased the AI Agent Token
// (`aat_`) lifecycle — that surface was retired ahead of a redesign,
// so the demo focuses on the session_token half instead.)
test("walk: opaque session token lifecycle", async ({ page, request }) => {
    // --- 1. Login -----------------------------------------------------
    await page.goto("/login");
    await page.evaluate(() => localStorage.clear());
    await page.goto("/login");
    await pause(page, 600);
    await caption(
        page,
        "Phase 2 auth: a single opaque session token replaces the old JWT pair.",
        1500,
    );

    await page.getByLabel("Server URL", { exact: true }).fill(backendURL);
    await page.getByLabel("Username", { exact: true }).fill(ADMIN_USERNAME);
    await page.getByLabel("Password", { exact: true }).fill(ADMIN_PASSWORD);
    await caption(page, "Logging in as admin…", 800);
    await page.getByRole("button", { name: "Log in", exact: true }).click();
    await expect(page).toHaveURL(/\/projects/, { timeout: 15_000 });
    await pause(page, 600);

    // The frontend persists the session_token in localStorage under
    // `platypus.sessions`. Pull it out so the caption can render the
    // pst_ prefix on screen.
    const session = await page.evaluate(() => {
        const raw = localStorage.getItem("platypus.sessions");
        if (!raw) return null;
        const obj = JSON.parse(raw) as Record<string, { sessionToken?: string }>;
        const first = Object.values(obj)[0];
        return first?.sessionToken ?? null;
    });
    expect(session).toBeTruthy();
    expect(session!.startsWith("pst_")).toBe(true);
    await caption(
        page,
        `Session bearer: ${session!.slice(0, 16)}…  (no refresh_token, no JWT)`,
        2200,
    );

    // --- 2. Use the bearer against a protected route -----------------
    await caption(page, "Calling /api/v1/projects with the session bearer…", 1500);
    const projResp = await request.get(`${backendURL}/api/v1/projects`, {
        headers: { Authorization: `Bearer ${session}` },
    });
    expect(projResp.status()).toBe(200);
    await caption(page, "200 OK — RBAC honoured the admin role.", 1500);

    // --- 3. Logout invalidates the bearer immediately ----------------
    await caption(page, "Logout — server-side revoke is synchronous…", 1500);
    const logoutResp = await request.post(`${backendURL}/api/v1/auth/logout`, {
        headers: { Authorization: `Bearer ${session}` },
    });
    expect([200, 204]).toContain(logoutResp.status());

    const afterLogout = await request.get(`${backendURL}/api/v1/projects`, {
        headers: { Authorization: `Bearer ${session}` },
    });
    expect(afterLogout.status()).toBe(401);
    await caption(
        page,
        "Same bearer → 401 immediately. No 30s cache lag on revoked sessions.",
        2200,
    );

    await pause(page, 800);
    await clearOverlays(page);
});
