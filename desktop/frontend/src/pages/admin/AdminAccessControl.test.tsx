import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

vi.mock("../../lib/auth", () => ({
    getSessionUser: () => ({
        id: "u-test",
        username: "ada",
        role: "admin" as const,
    }),
}));

const listRBACRoles = vi.fn();
const listRBACPermissions = vi.fn();
const getRBACRole = vi.fn();
const createRBACRole = vi.fn();
const updateRBACRole = vi.fn();
const deleteRBACRole = vi.fn();

vi.mock("../../lib/api", () => ({
    listRBACRoles: (...args: unknown[]) => listRBACRoles(...args),
    listRBACPermissions: (...args: unknown[]) => listRBACPermissions(...args),
    getRBACRole: (...args: unknown[]) => getRBACRole(...args),
    createRBACRole: (...args: unknown[]) => createRBACRole(...args),
    updateRBACRole: (...args: unknown[]) => updateRBACRole(...args),
    deleteRBACRole: (...args: unknown[]) => deleteRBACRole(...args),
}));

import AdminAccessControl from "./AdminAccessControl";

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("<AdminAccessControl>", () => {
    it("renders Roles / Permissions tabs", () => {
        listRBACRoles.mockResolvedValue([]);
        listRBACPermissions.mockResolvedValue([]);
        renderInRouter(<AdminAccessControl />);
        expect(screen.getByTestId("access-control-tabs")).toBeInTheDocument();
        expect(screen.getByRole("tab", { name: /roles/i })).toBeInTheDocument();
        expect(screen.getByRole("tab", { name: /permissions/i })).toBeInTheDocument();
    });

    it("Roles tab loads roles on mount and shows builtins", async () => {
        listRBACRoles.mockResolvedValue([
            {
                slug: "viewer", name: "Viewer", is_builtin: true,
                is_global: true, is_project: true,
                created_at: "", updated_at: "",
            },
            {
                slug: "operator", name: "Operator", is_builtin: true,
                is_global: true, is_project: true,
                created_at: "", updated_at: "",
            },
        ]);
        listRBACPermissions.mockResolvedValue([]);
        renderInRouter(<AdminAccessControl />);
        await waitFor(() => expect(listRBACRoles).toHaveBeenCalled());
        expect(screen.getByText(/^Viewer$/)).toBeInTheDocument();
        expect(screen.getByText(/^Operator$/)).toBeInTheDocument();
    });

    it("switching to Permissions tab loads catalogue", async () => {
        const user = userEvent.setup();
        listRBACRoles.mockResolvedValue([]);
        listRBACPermissions.mockResolvedValue([
            { slug: "hosts:read", resource: "hosts", description: "List hosts." },
            { slug: "hosts:exec", resource: "hosts", description: "Run commands." },
        ]);
        renderInRouter(<AdminAccessControl />);
        await user.click(screen.getByRole("tab", { name: /permissions/i }));
        await waitFor(() => expect(listRBACPermissions).toHaveBeenCalled());
        expect(screen.getByText("hosts:read")).toBeInTheDocument();
        expect(screen.getByText("hosts:exec")).toBeInTheDocument();
    });

    it("Issue 'New role' button opens the create dialog", async () => {
        const user = userEvent.setup();
        listRBACRoles.mockResolvedValue([]);
        listRBACPermissions.mockResolvedValue([
            { slug: "hosts:read", resource: "hosts", description: "List hosts." },
        ]);
        renderInRouter(<AdminAccessControl />);
        await user.click(screen.getByRole("button", { name: /new role/i }));
        // Dialog with slug + name fields appears.
        expect(screen.getByLabelText(/slug/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/name/i)).toBeInTheDocument();
    });
});
