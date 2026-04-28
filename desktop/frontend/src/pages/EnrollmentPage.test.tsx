import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

// EnrollmentPage's user-facing copy was rewritten to drop the bare "PAT"
// acronym and the "access token" framing. The token tab now reads as
// "Enrollment tokens", since these tokens exist solely to let an agent
// JOIN A FLEET — they are not user-account API tokens (a separate,
// future-tense feature). The new vocabulary makes the scope obvious so
// engineers don't conflate them with GitHub-style PATs.
//
// What this spec pins:
//   1. The second tab label is "Enrollment tokens" (not "Access tokens (PAT)").
//   2. The page subtitle no longer leads with "raw access tokens".
//   3. The headline "Issue …" button on the tokens tab uses the new noun.
//   4. The issuance dialog title uses the new noun.
//   5. The "issued" success dialog uses the new noun.

vi.mock("../layout/ProjectShell", () => ({
    useCurrentProject: () => ({
        id: "p1",
        slug: "test-project",
        name: "Test Project",
    }),
}));

vi.mock("../lib/api", () => ({
    listInstallArtifacts: vi.fn().mockResolvedValue([]),
    listInstallPlatforms: vi.fn().mockResolvedValue({
        channel: "stable",
        platforms: [{ os: "linux", arch: "amd64" }],
    }),
    listPATTokens: vi.fn().mockResolvedValue([]),
    issueInstallArtifact: vi.fn(),
    issuePAT: vi.fn().mockResolvedValue({ token_id: "tok_1", token: "plt_xxx" }),
    revokeInstallArtifact: vi.fn(),
    revokePAT: vi.fn(),
    getServerInfo: vi.fn().mockResolvedValue({
        server_endpoint: "127.0.0.1:7332",
        version: "test",
        commit: "test",
    }),
}));

import EnrollmentPage from "./EnrollmentPage";

function renderPage() {
    return render(
        <MemoryRouter>
            <EnrollmentPage />
        </MemoryRouter>,
    );
}

describe("<EnrollmentPage> vocabulary — enrollment tokens (not access tokens)", () => {
    it("labels the second tab 'Enrollment tokens'", async () => {
        renderPage();
        const tab = await screen.findByRole("tab", { name: /enrollment tokens/i });
        expect(tab).toBeInTheDocument();
        // The old "Access tokens" copy must be gone from the tab list so
        // future grep / docs searches don't find a stale label.
        expect(
            screen.queryByRole("tab", { name: /access tokens/i }),
        ).not.toBeInTheDocument();
    });

    it("page subtitle uses 'enrollment tokens', not 'access tokens'", () => {
        const { container } = renderPage();
        const text = container.textContent ?? "";
        expect(text.toLowerCase()).toContain("enrollment tokens");
        expect(text.toLowerCase()).not.toContain("access tokens");
    });

    it("'Issue enrollment token' button is reachable from the tokens tab", async () => {
        const user = userEvent.setup();
        renderPage();
        const tab = await screen.findByRole("tab", { name: /enrollment tokens/i });
        await user.click(tab);
        const button = await screen.findByRole("button", {
            name: /issue (an? )?enrollment token/i,
        });
        expect(button).toBeInTheDocument();
        // No leftover "Issue access token" labelling.
        expect(
            screen.queryByRole("button", { name: /issue (an? )?access token/i }),
        ).not.toBeInTheDocument();
    });

    it("the issue dialog title leads with 'enrollment token'", async () => {
        const user = userEvent.setup();
        renderPage();
        await user.click(await screen.findByRole("tab", { name: /enrollment tokens/i }));
        await user.click(
            await screen.findByRole("button", {
                name: /issue (an? )?enrollment token/i,
            }),
        );
        // Radix dialogs render their <DialogTitle> with role="heading"
        // inside role="dialog". Match against that to avoid coupling to
        // tag names.
        const dialogHeading = await screen.findByRole("heading", {
            name: /enrollment token/i,
        });
        expect(dialogHeading).toBeInTheDocument();
        expect(dialogHeading.textContent ?? "").not.toMatch(/access token/i);
    });
});
