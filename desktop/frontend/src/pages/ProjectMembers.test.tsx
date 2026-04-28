import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { renderWithQueryClient } from "../testing/renderWithQueryClient";

vi.mock("../lib/api", () => ({
    listProjectMembers: vi.fn().mockResolvedValue([
        { user_id: "u1", username: "ada", role: "admin" },
    ]),
    listUsers: vi.fn().mockResolvedValue([
        { id: "u2", username: "bob", role: "operator" },
    ]),
    addProjectMember: vi.fn().mockResolvedValue(undefined),
    removeProjectMember: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("../lib/auth", () => ({
    getSessionUser: () => ({
        id: "u-admin",
        username: "ada",
        role: "admin" as const,
    }),
}));

import ProjectMembers from "./ProjectMembers";

const project = {
    id: "p1",
    slug: "test",
    name: "Test",
    created_at: 0,
};

// The Add member dialog used to have a single "Add" button that
// always closed the dialog. Onboarding a fresh project (5 people)
// turned into 5 round-trips through the trigger button + dropdown.
// "Add another" submits the same form but keeps the dialog open and
// resets the fields so chained adds are one-handed.

describe("<ProjectMembers> add-member dialog", () => {
    it("exposes both 'Add' and 'Add another' submit buttons", async () => {
        const user = userEvent.setup();
        renderWithQueryClient(<ProjectMembers project={project as never} />);

        // Wait for the initial load and click the "Add member"
        // trigger; jsdom resolves the async members fetch in a
        // microtask so the button is reachable on the next tick.
        const trigger = await screen.findByRole("button", { name: /add member/i });
        await user.click(trigger);

        // Both submit buttons live inside the dialog. Match exact
        // labels so the trigger button doesn't collide with the
        // dialog's "Add" submit.
        expect(
            await screen.findByRole("button", { name: /^add$/i }),
        ).toBeInTheDocument();
        expect(
            screen.getByRole("button", { name: /^add another$/i }),
        ).toBeInTheDocument();
    });
});
