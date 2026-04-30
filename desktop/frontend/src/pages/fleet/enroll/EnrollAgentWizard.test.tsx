import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

import { renderWithQueryClient } from "../../../testing/renderWithQueryClient";

// Radix Select uses Pointer Events APIs that jsdom doesn't implement
// (hasPointerCapture / releasePointerCapture / scrollIntoView). The
// downloader-picker test below opens the dropdown and clicks an
// option, which exercises all three. Stubbing once at module load
// time keeps the test ergonomic without dragging in a heavier
// pointer-events polyfill.
beforeAll(() => {
    const proto = Element.prototype as unknown as Record<string, unknown>;
    if (typeof proto.hasPointerCapture !== "function") {
        proto.hasPointerCapture = () => false;
        proto.releasePointerCapture = () => {};
        proto.setPointerCapture = () => {};
    }
    if (typeof proto.scrollIntoView !== "function") {
        proto.scrollIntoView = () => {};
    }
});

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

    // Multi-downloader picker: when the server returns a populated
    // install_commands map, the RunStep dropdown lets the operator
    // switch between variants without re-issuing the (single-use)
    // install token. Pinned because the macOS LibreSSL fix relies on
    // this switching working — the operator may need to bail off
    // curl onto wget / python3 to get past a broken default tool.
    it("switches the install command when the downloader picker changes", async () => {
        issueInstallArtifact.mockResolvedValue({
            download_id: "dl_y",
            download_token: "dl_y.secret",
            expires_at: new Date(Date.now() + 60_000).toISOString(),
            server_endpoint: "203.0.113.5:13337",
            install_command: "curl-cmd",
            install_commands: {
                curl: "curl-cmd",
                wget: "wget-cmd",
                python3: "py-cmd",
            },
            bundle_url: "https://example.test/install/dl_y",
        });
        const user = userEvent.setup();
        render("/projects/test-project/hosts?enroll=1");

        await screen.findByTestId("enroll-wizard-os");
        await user.click(screen.getByTestId("enroll-wizard-next"));
        await user.click(screen.getByTestId("enroll-wizard-next"));
        await screen.findByTestId("enroll-wizard-connect");
        await user.click(screen.getByTestId("enroll-wizard-submit"));

        await waitFor(() =>
            expect(screen.getByTestId("enroll-wizard-run")).toBeInTheDocument(),
        );

        // Default selection renders the curl variant.
        expect(screen.getByText("curl-cmd")).toBeInTheDocument();

        // Open the picker (Radix Select renders the trigger as a
        // combobox), pick the wget option.
        const triggers = screen.getAllByTestId("downloader-picker");
        await user.click(triggers[0]);
        const wgetOption = await screen.findByRole("option", { name: "wget" });
        await user.click(wgetOption);

        // Code block now shows the wget command and the curl one is gone.
        await waitFor(() =>
            expect(screen.getByText("wget-cmd")).toBeInTheDocument(),
        );
        expect(screen.queryByText("curl-cmd")).not.toBeInTheDocument();
    });

    // Skip-TLS-verification toggle: defaults ON so the wizard shows
    // the insecure flavour (matches first-boot self-signed servers),
    // and toggling it OFF swaps the rendered command to the strict
    // map the server returned. Both maps survive the same install
    // token so no re-issue happens.
    it("toggles between insecure and strict install commands", async () => {
        issueInstallArtifact.mockResolvedValue({
            download_id: "dl_z",
            download_token: "dl_z.secret",
            expires_at: new Date(Date.now() + 60_000).toISOString(),
            server_endpoint: "203.0.113.5:13337",
            install_command: "curl-insecure",
            install_commands: {
                curl: "curl-insecure",
                wget: "wget-insecure",
            },
            install_commands_strict: {
                curl: "curl-strict",
                wget: "wget-strict",
            },
            bundle_url: "https://example.test/install/dl_z",
        });
        const user = userEvent.setup();
        render("/projects/test-project/hosts?enroll=1");

        await screen.findByTestId("enroll-wizard-os");
        await user.click(screen.getByTestId("enroll-wizard-next"));
        await user.click(screen.getByTestId("enroll-wizard-next"));
        await screen.findByTestId("enroll-wizard-connect");
        await user.click(screen.getByTestId("enroll-wizard-submit"));

        await waitFor(() =>
            expect(screen.getByTestId("enroll-wizard-run")).toBeInTheDocument(),
        );

        // Default: toggle is on, insecure command is rendered.
        expect(screen.getByText("curl-insecure")).toBeInTheDocument();
        expect(screen.queryByText("curl-strict")).not.toBeInTheDocument();

        // Toggle off → strict command takes over.
        const toggles = screen.getAllByTestId("skip-tls-toggle");
        await user.click(toggles[0]);
        await waitFor(() =>
            expect(screen.getByText("curl-strict")).toBeInTheDocument(),
        );
        expect(screen.queryByText("curl-insecure")).not.toBeInTheDocument();
    });

    // Bundle commands now ship in the API response (used to be
    // composed FE-side via bundleOneLinerFor). Verify the wizard
    // reads from result.bundle_commands and that the same
    // downloader picker drives both the script and bundle tabs.
    it("renders server-supplied bundle commands in the bundle tab", async () => {
        issueInstallArtifact.mockResolvedValue({
            download_id: "dl_b",
            download_token: "dl_b.secret",
            expires_at: new Date(Date.now() + 60_000).toISOString(),
            server_endpoint: "203.0.113.5:13337",
            install_command: "curl-script-insecure",
            install_commands: {
                curl: "curl-script-insecure",
                wget: "wget-script-insecure",
            },
            install_commands_strict: {
                curl: "curl-script-strict",
                wget: "wget-script-strict",
            },
            bundle_commands: {
                curl: "curl-bundle-insecure",
                wget: "wget-bundle-insecure",
            },
            bundle_commands_strict: {
                curl: "curl-bundle-strict",
                wget: "wget-bundle-strict",
            },
            bundle_url: "https://example.test/install/dl_b?format=bundle",
        });
        const user = userEvent.setup();
        render("/projects/test-project/hosts?enroll=1");

        await screen.findByTestId("enroll-wizard-os");
        await user.click(screen.getByTestId("enroll-wizard-next"));
        await user.click(screen.getByTestId("enroll-wizard-next"));
        await screen.findByTestId("enroll-wizard-connect");
        await user.click(screen.getByTestId("enroll-wizard-submit"));

        await waitFor(() =>
            expect(screen.getByTestId("enroll-wizard-run")).toBeInTheDocument(),
        );

        // Switch to the bundle tab.
        await user.click(screen.getByRole("tab", { name: "offline bundle" }));
        // Bundle tab shows the curl bundle (default downloader = curl, toggle on).
        await waitFor(() =>
            expect(screen.getByText("curl-bundle-insecure")).toBeInTheDocument(),
        );
        // Toggle off → strict bundle.
        const toggles = screen.getAllByTestId("skip-tls-toggle");
        await user.click(toggles[0]);
        await waitFor(() =>
            expect(screen.getByText("curl-bundle-strict")).toBeInTheDocument(),
        );
    });
});
