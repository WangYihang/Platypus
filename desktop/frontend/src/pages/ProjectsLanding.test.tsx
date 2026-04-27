import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

let useShellMock = vi.fn();
let getSessionUserMock = vi.fn();

vi.mock("../layout/ProjectShell", () => ({
    useShell: () => useShellMock(),
}));

vi.mock("../lib/auth", () => ({
    getSessionUser: () => getSessionUserMock(),
}));

import ProjectsLanding from "./ProjectsLanding";

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

// The empty state on /projects used to read "An admin creates projects
// from the sidebar." That copy is accurate for admins (who DO see a
// New project button) but unhelpful for viewers/operators — they
// don't have the button and reading "an admin creates them" leaves
// them stuck. Branch the copy on role so each audience reads
// something they can act on.

describe("<ProjectsLanding> empty state", () => {
    it("tells admins they can create projects from the sidebar", () => {
        useShellMock.mockReturnValue({
            projects: [],
            project: null,
            refresh: vi.fn(),
            loading: false,
        });
        getSessionUserMock.mockReturnValue({
            id: "u-admin",
            username: "ada",
            role: "admin",
        });
        renderInRouter(<ProjectsLanding />);
        expect(
            screen.getByText(/create projects from the sidebar/i),
        ).toBeInTheDocument();
    });

    it("tells operators / viewers to ask their admin", () => {
        useShellMock.mockReturnValue({
            projects: [],
            project: null,
            refresh: vi.fn(),
            loading: false,
        });
        getSessionUserMock.mockReturnValue({
            id: "u-bob",
            username: "bob",
            role: "operator",
        });
        renderInRouter(<ProjectsLanding />);
        expect(
            screen.getByText(/ask your admin/i),
        ).toBeInTheDocument();
        expect(
            screen.queryByText(/create projects from the sidebar/i),
        ).toBeNull();
    });
});
