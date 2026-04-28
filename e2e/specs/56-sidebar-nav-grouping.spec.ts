import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The project sidebar used to render six nav items as a flat list:
// Overview / Fleet / Activities / Enrollment / Members / Settings.
// New users had to learn six labels with no hint at the relationship
// between them — "is Activities the audit log? does it overlap with
// Members? where do I add a host?". Plan section 5 split them into
// labelled groups so the IA reads at a glance.
//
// Activities was originally bucketed under WORK, but it's read-only
// history — operators don't *do* anything there. It now lives in its
// own AUDIT group, leaving room for future audit-only surfaces
// (sessions log, command log, …) without expanding Work.
//
// Step 1 of the settings reorg: ENROLLMENT no longer lives under
// ADMIN as a peer of Members. The "PAT" tokens it issues are not
// account-level credentials — they're one-shot agent-bootstrap
// secrets. So the surface moved INSIDE Fleet (it's how you grow the
// fleet), reachable from FleetPage's header. ADMIN now contains
// only Members.
//
// 2026-04 IA pass: AUDIT collapsed Activities + Recordings + Transfers
// into one "Audit" entry that opens AuditPage with internal tabs. The
// group keeps a single item so a future second audit kind only needs
// to push one new entry into the same group.
//
//   WORK
//     · Overview      (dashboard)
//     · Fleet         (hosts + sessions + topology + enroll)
//   ADMIN
//     · Members       (who can see this project)
//   AUDIT
//     · Audit         (activities / recordings / transfers tabs)
//   PROJECT
//     · Settings      (project-level config)
//
// We test for the GROUP HEADERS (so the visual grouping is intact)
// and the order (so a future "let me reorder these" doesn't sneak
// Settings above Fleet without anyone noticing).
test.describe("project sidebar nav grouping", () => {
    test("renders WORK / ADMIN / AUDIT / PROJECT groups with correct items in order", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/overview");

        // Group headings appear as small caps labels above each
        // section. Tagged with data-testid="nav-group-<key>" so the
        // spec doesn't depend on letter-case styling.
        const groupWork = page.getByTestId("nav-group-work");
        const groupAdmin = page.getByTestId("nav-group-admin");
        const groupAudit = page.getByTestId("nav-group-audit");
        const groupProject = page.getByTestId("nav-group-project");

        await expect(groupWork).toBeVisible({ timeout: 10_000 });
        await expect(groupAdmin).toBeVisible();
        await expect(groupAudit).toBeVisible();
        await expect(groupProject).toBeVisible();

        // Check the labels are recognisable.
        expect((await groupWork.textContent())?.trim().toLowerCase()).toContain("work");
        expect((await groupAdmin.textContent())?.trim().toLowerCase()).toContain("admin");
        expect((await groupAudit.textContent())?.trim().toLowerCase()).toContain("audit");
        expect((await groupProject.textContent())?.trim().toLowerCase()).toContain(
            "project",
        );

        // Each group's data-testid="nav-group-items-<key>" holds the
        // ordered list of NavLinks underneath it. Walk them and
        // assert the labels appear in the documented order.
        async function itemsOf(
            key: "work" | "admin" | "audit" | "project",
        ): Promise<string[]> {
            const list = page.getByTestId(`nav-group-items-${key}`);
            await expect(list).toBeVisible();
            const links = list.locator(".pl-nav-link");
            const count = await links.count();
            const out: string[] = [];
            for (let i = 0; i < count; i++) {
                out.push(((await links.nth(i).textContent()) ?? "").trim());
            }
            return out;
        }

        expect(await itemsOf("work")).toEqual(["Overview", "Fleet"]);
        expect(await itemsOf("admin")).toEqual(["Members"]);
        expect(await itemsOf("audit")).toEqual(["Audit"]);
        expect(await itemsOf("project")).toEqual(["Settings"]);
    });
});
