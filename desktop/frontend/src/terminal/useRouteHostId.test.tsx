import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";

import { HOST_SUBVIEWS } from "@/lib/hostRoutes";
import { useRouteHostId } from "./useRouteHostId";

// The terminal drawer scopes its shells with this hook, from a spot in
// the tree above the :hostId route — so useParams sees nothing and the
// answer has to come from matching the whole location.
//
// The failure this guards is quiet: /hosts/topology matches the same
// pattern /hosts/:hostId does, so a sub-view read as a host id gives
// the drawer a host that can never exist.
function Probe() {
    const hostId = useRouteHostId();
    return <span data-testid="host-id">{hostId ?? "(none)"}</span>;
}

function renderAt(path: string) {
    return render(
        <MemoryRouter initialEntries={[path]}>
            <Routes>
                <Route path="/*" element={<Probe />} />
            </Routes>
        </MemoryRouter>,
    );
}

describe("useRouteHostId", () => {
    it("reads :hostId from a host detail URL at any depth", () => {
        renderAt("/projects/demo/hosts/h-123/files");
        expect(screen.getByTestId("host-id")).toHaveTextContent("h-123");
    });

    it("reads :hostId from the bare host URL", () => {
        renderAt("/projects/demo/hosts/h-456");
        expect(screen.getByTestId("host-id")).toHaveTextContent("h-456");
    });

    it.each([...HOST_SUBVIEWS])("does not treat /hosts/%s as a host", (view) => {
        renderAt(`/projects/demo/hosts/${view}`);
        expect(screen.getByTestId("host-id")).toHaveTextContent("(none)");
    });

    it("returns nothing off the hosts routes entirely", () => {
        renderAt("/projects/demo/activity");
        expect(screen.getByTestId("host-id")).toHaveTextContent("(none)");
    });

    // it.each over an empty list silently passes, which would make the
    // sub-view case above vacuous.
    it("has sub-views to assert over", () => {
        expect(HOST_SUBVIEWS.length).toBeGreaterThan(0);
    });
});
