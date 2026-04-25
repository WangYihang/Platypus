import { expect, test } from "@playwright/test";

import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../../fixtures/env";
import { caption, clearOverlays, pause } from "../../fixtures/demo";

// 06-auth-redesign — narrated end-to-end demonstration of the Phase 1+2
// auth redesign: opaque session_token (pst_) for humans + opaque AAT
// (aat_) for AI agents. Walks through:
//   1. Login UI → server returns a single session_token, no JWT pair
//   2. Mint an AAT via /api/v1/aat with admin's session bearer
//   3. Use that AAT to hit a protected endpoint (/api/v1/projects)
//   4. Revoke the AAT — verifier cache is invalidated synchronously
//   5. Same AAT immediately fails (401)
//
// The AAT lifecycle is API-only (no UI surface yet), so the demo
// captions narrate the underlying state for the viewer while the
// browser shows the LocalStorage entry that holds the session token.
test("walk: opaque session + AAT lifecycle", async ({ page, request }) => {
    // --- 1. Login (Phase 2: pst_ session token) ---------------------
    await page.goto("/login");
    await page.evaluate(() => localStorage.clear());
    await page.goto("/login");
    await pause(page, 600);
    await caption(page, "Phase 2 auth: a single opaque session token replaces the old JWT pair.", 1500);

    await page.getByLabel("Server URL", { exact: true }).fill(backendURL);
    await page.getByLabel("Username", { exact: true }).fill(ADMIN_USERNAME);
    await page.getByLabel("Password", { exact: true }).fill(ADMIN_PASSWORD);
    await caption(page, "Logging in as admin…", 800);
    await page.getByRole("button", { name: "Log in", exact: true }).click();
    await expect(page).toHaveURL(/\/projects/, { timeout: 15_000 });
    await pause(page, 600);

    // The frontend persists the session_token in localStorage; show
    // its prefix on screen so viewers see the pst_ shape and confirm
    // there is no separate refresh token field.
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
        2000,
    );

    // --- 2. Mint an AAT --------------------------------------------
    // The admin session is what authorises this — the same bearer
    // browser code uses for any other authenticated REST call.
    await caption(page, "Minting an AI Agent Token (aat_) for an LLM caller…", 1500);
    const mintResp = await request.post(`${backendURL}/api/v1/aat`, {
        headers: { Authorization: `Bearer ${session}` },
        data: {
            name: "demo-agent",
            role: "viewer",
            scopes: ["hosts:read", "projects:read"],
            ttl_seconds: 3600,
        },
    });
    expect(mintResp.status()).toBe(201);
    const aat = (await mintResp.json()) as {
        token_id: string;
        token: string;
        scopes: string[];
    };
    expect(aat.token.startsWith("aat_")).toBe(true);
    await caption(
        page,
        `AAT issued: ${aat.token_id} — scopes: ${aat.scopes.join(", ")}`,
        2200,
    );

    // --- 3. Use the AAT against a protected route ------------------
    await caption(page, "Calling /api/v1/projects with the AAT bearer…", 1400);
    const projResp = await request.get(`${backendURL}/api/v1/projects`, {
        headers: { Authorization: `Bearer ${aat.token}` },
    });
    expect(projResp.status()).toBe(200);
    await caption(page, "200 OK — verifier resolved the aat_, RBAC honoured projects:read.", 1800);

    // --- 4. Revoke + verify cache invalidates synchronously --------
    await caption(page, "Revoking the AAT — cache invalidate is synchronous…", 1500);
    const revokeResp = await request.delete(`${backendURL}/api/v1/aat/${aat.token_id}`, {
        headers: { Authorization: `Bearer ${session}` },
    });
    expect(revokeResp.status()).toBe(204);

    const afterRevoke = await request.get(`${backendURL}/api/v1/projects`, {
        headers: { Authorization: `Bearer ${aat.token}` },
    });
    expect(afterRevoke.status()).toBe(401);
    await caption(page, "Same bearer → 401 immediately. No 30s cache lag for revoked tokens.", 2200);

    await pause(page, 800);
    await clearOverlays(page);
});
