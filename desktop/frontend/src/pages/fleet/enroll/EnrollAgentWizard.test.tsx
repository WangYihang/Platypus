import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

import { renderWithQueryClient } from "../../../testing/renderWithQueryClient";

// EnrollAgentWizard's open / closed state is driven by `?enroll=1`
// on the URL. Mounting with that param therefore opens the wizard
// directly. Each test below lands on a fresh URL so the wizard
// state can be exercised in isolation.

vi.mock("../../../layout/ProjectShell", () => ({
    useCurrentProject: () => ({
        id: "p1",
        slug: "test-project",
        name: "Test Project",
    }),
}));

const issueInstallArtifact = vi.fn();
const listInstallPlatforms = vi.fn();
const getServerInfo = vi.fn();

vi.mock("../../../lib/api", () => ({
    issueInstallArtifact: (...args: unknown[]) => issueInstallArtifact(...args),
    listInstallPlatforms: () => listInstallPlatforms(),
    getServerInfo: () => getServerInfo(),
}));

import EnrollAgentWizard from "./EnrollAgentWizard";

function render(initialUrl: string) {
    return renderWithQueryClient(
        <MemoryRouter initialEntries={[initialUrl]}>
            <EnrollAgentWizard />
        </MemoryRouter>,
    );
}

beforeEach(() => {
    issueInstallArtifact.mockReset();
    listInstallPlatforms.mockReset();
    getServerInfo.mockReset();
    listInstallPlatforms.mockResolvedValue({
        channel: "stable",
        platforms: [
            { os: "linux", arch: "amd64" },
            { os: "linux", arch: "arm64" },
        ],
    });
    getServerInfo.mockResolvedValue({ public_addr: "203.0.113.5:13337" });
});

describe("<EnrollAgentWizard>", () => {
    it("stays closed without ?enroll=1", () => {
        render("/projects/test-project/hosts");
        expect(screen.queryByTestId("enroll-wizard")).not.toBeInTheDocument();
    });

    it("opens with ?enroll=1 and lands on the OS step", async () => {
        render("/projects/test-project/hosts?enroll=1");
        expect(await screen.findByTestId("enroll-wizard")).toBeInTheDocument();
        // Step indicator marks `os` as the active step.
        const indicator = await screen.findByTestId("enroll-wizard-steps");
        const active = indicator.querySelector('[data-step="os"]');
        expect(active?.getAttribute("data-active")).toBe("true");
    });

    it("walks through all four steps and submits with skipped OS/arch", async () => {
        issueInstallArtifact.mockResolvedValue({
            download_id: "dl_x",
            download_token: "dl_x.secret",
            expires_at: new Date(Date.now() + 60_000).toISOString(),
            server_endpoint: "203.0.113.5:13337",
            install_command: "curl -fsSL https://example.test/install/dl_x | sh",
            bundle_url: "https://example.test/install/dl_x",
        });
        const user = userEvent.setup();
        render("/projects/test-project/hosts?enroll=1");

        // Wait for the OS step to render its picker (depends on the
        // platforms fetch landing).
        await screen.findByTestId("enroll-wizard-os");

        // Skip OS — click Next.
        await user.click(screen.getByTestId("enroll-wizard-next"));
        // Arch step renders the "no OS selected" hint, no toggle.
        expect(screen.queryByTestId("enroll-wizard-arch")).not.toBeInTheDocument();

        // Skip arch — click Next.
        await user.click(screen.getByTestId("enroll-wizard-next"));
        // Connect step renders.
        const connect = await screen.findByTestId("enroll-wizard-connect");
        expect(connect).toBeInTheDocument();

        // Server endpoint should be prefilled from getServerInfo.
        const endpoint = connect.querySelector(
            'input[placeholder="203.0.113.5:13337"]',
        ) as HTMLInputElement;
        expect(endpoint.value).toBe("203.0.113.5:13337");

        // Submit the connect step → wizard advances to run.
        await user.click(screen.getByTestId("enroll-wizard-submit"));

        await waitFor(() =>
            expect(screen.getByTestId("enroll-wizard-run")).toBeInTheDocument(),
        );
        // Both target_os and target_arch are absent from the request
        // payload when the operator skipped them.
        expect(issueInstallArtifact).toHaveBeenCalledTimes(1);
        const [, body] = issueInstallArtifact.mock.calls[0];
        expect(body.target_os).toBeUndefined();
        expect(body.target_arch).toBeUndefined();
        // Result step renders the install command verbatim.
        expect(
            screen.getByText(
                "curl -fsSL https://example.test/install/dl_x | sh",
            ),
        ).toBeInTheDocument();
    });
});
