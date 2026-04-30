import { describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";

vi.mock("../../layout/ProjectShell", () => ({
    useCurrentProject: () => ({
        id: "p1",
        slug: "test-project",
        name: "Test Project",
    }),
}));

vi.mock("./HostsCardPanel", () => ({ default: () => <div /> }));
vi.mock("./HostsPanel", () => ({ default: () => <div /> }));

vi.mock("../../lib/api", () => ({
    listHosts: vi.fn().mockResolvedValue([]),
}));

import HostsView from "./HostsView";
import { writePreference } from "../../lib/preferences";
import { renderWithQueryClient } from "../../testing/renderWithQueryClient";

// HostsView owns the Cards / Table view toggle that used to live in
// FleetPage. The toggle is annotated with a title= attribute that
// names the user's stored default-view preference and points at
// /preferences as the place to change it. Without this hint, switching
// projects could land on an unexpected default with no clue why.

describe("<HostsView>", () => {
    it("annotates the view toggle with the current default-view preference", () => {
        writePreference("ui.fleet.defaultView", "cards");
        const { container } = renderWithQueryClient(
            <MemoryRouter initialEntries={["/projects/test-project/hosts"]}>
                <Routes>
                    <Route
                        path="/projects/:projectSlug/hosts"
                        element={<HostsView />}
                    />
                </Routes>
            </MemoryRouter>,
        );
        const toggle = container.querySelector(
            '[data-testid="fleet-view-toggle"]',
        );
        expect(toggle).not.toBeNull();
        const title = toggle!.getAttribute("title") ?? "";
        expect(title).toMatch(/default view/i);
        expect(title).toMatch(/cards/i);
        expect(title).toMatch(/preferences/i);
    });
});
