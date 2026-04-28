import { describe, expect, it, vi } from "vitest";
import { render, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import ProjectSidebar from "./ProjectSidebar";
import type { Project } from "../lib/api";
import type { SessionUser } from "../lib/auth";

// "PAT" is misleading shorthand for what these tokens actually are —
// one-shot enrolment credentials that an agent burns to JOIN a fleet,
// after which mTLS takes over. So the entire Enrollment surface is
// reframed as part of FLEET (it's how you grow the fleet), not as a
// standalone admin verb.
//
// Concretely, this spec pins the post-reorg sidebar shape:
//
//   WORK    · Overview · Fleet
//   ADMIN   · Members            ← Enrollment NO LONGER lives here
//   AUDIT   · Activities
//   PROJECT · Settings
//
// A separate "Enroll agent" entry point lives inside FleetPage's
// header (covered by FleetPage.test.tsx), so the sidebar IA stays
// tight without losing the action.

vi.mock("../components/Brand", () => ({ default: () => <div /> }));
vi.mock("./CmdKHint", () => ({ default: () => <div /> }));
vi.mock("./ProjectSwitcher", () => ({ default: () => <div /> }));
vi.mock("./UserMenu", () => ({ default: () => <div /> }));

const adminUser: SessionUser = {
    id: "u-admin",
    username: "ada",
    role: "admin",
};

const project: Project = {
    id: "p1",
    slug: "test-project",
    name: "Test Project",
    created_at: "2024-01-01T00:00:00Z",
    created_by: "u-admin",
};

function renderSidebar() {
    return render(
        <MemoryRouter initialEntries={["/projects/test-project/overview"]}>
            <ProjectSidebar
                user={adminUser}
                serverURL="http://localhost"
                projects={[project]}
                currentSlug="test-project"
                onProjectsChanged={() => {}}
                onAddServer={() => {}}
                onManageServers={() => {}}
            />
        </MemoryRouter>,
    );
}

describe("<ProjectSidebar> — Enrollment is no longer in the Admin group", () => {
    it("ADMIN group renders only Members", () => {
        const { getByTestId } = renderSidebar();
        const adminItems = getByTestId("nav-group-items-admin");
        const links = within(adminItems).getAllByRole("link");
        const labels = links.map((l) => (l.textContent ?? "").trim());
        expect(labels).toEqual(["Members"]);
    });

    it("no nav link points at the standalone /enrollment path", () => {
        const { container } = renderSidebar();
        const all = container.querySelectorAll<HTMLAnchorElement>("a[href]");
        const hrefs = Array.from(all).map((a) => a.getAttribute("href") ?? "");
        // The old `/projects/<slug>/enrollment` URL must not be linked
        // from the sidebar — Enrollment is reached from inside Fleet now.
        expect(hrefs.some((h) => h.endsWith("/enrollment"))).toBe(false);
    });

    it("WORK group still has Overview + Fleet, in that order", () => {
        const { getByTestId } = renderSidebar();
        const workItems = getByTestId("nav-group-items-work");
        const links = within(workItems).getAllByRole("link");
        const labels = links.map((l) => (l.textContent ?? "").trim());
        expect(labels).toEqual(["Overview", "Fleet"]);
    });
});
