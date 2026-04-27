import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

// ProjectSettings used to host four tabs — Identity (project-scoped) and
// three browser-local tabs (Display / Terminal / Behaviour). The
// browser-local trio actually has nothing to do with the active project
// (they live in localStorage and don't reset on project switch), so they
// moved to /preferences. ProjectSettings is now a single-purpose
// project-scoped surface: project identity + danger zone, no browser
// preferences leaking in.

vi.mock("../layout/ProjectShell", () => {
    return {
        useCurrentProject: () => ({
            id: "p1",
            slug: "test-project",
            name: "Test Project",
        }),
        useShell: () => ({
            projects: [],
            project: { id: "p1", slug: "test-project", name: "Test Project" },
            refresh: vi.fn(),
            loading: false,
        }),
    };
});

import ProjectSettings from "./ProjectSettings";

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("<ProjectSettings>", () => {
    it("renders the Identity card with project metadata", () => {
        renderInRouter(<ProjectSettings />);
        expect(screen.getByText(/identity/i)).toBeInTheDocument();
        // The slug appears twice: once in the PageHeader subtitle
        // ("Project · test-project") and once in the Identity card's
        // Mono. Just assert at least one match.
        expect(screen.getAllByText(/test-project/i).length).toBeGreaterThan(0);
    });

    it("renders the Danger zone with a Delete project button", () => {
        renderInRouter(<ProjectSettings />);
        expect(screen.getByText(/danger zone/i)).toBeInTheDocument();
        expect(
            screen.getByRole("button", { name: /delete project/i }),
        ).toBeInTheDocument();
    });

    it("does NOT render Display, Terminal, or Behaviour tabs (those moved to /preferences)", () => {
        renderInRouter(<ProjectSettings />);
        // After Batch 0, browser-local prefs no longer live here. A
        // regression that brings them back would re-introduce the
        // scope confusion the user explicitly called out.
        expect(screen.queryByRole("tab", { name: /display/i })).toBeNull();
        expect(screen.queryByRole("tab", { name: /^terminal$/i })).toBeNull();
        expect(screen.queryByRole("tab", { name: /behaviour/i })).toBeNull();
    });
});
