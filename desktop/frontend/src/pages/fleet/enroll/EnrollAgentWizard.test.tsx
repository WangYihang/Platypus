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
const listEnrollmentPresets = vi.fn();
const seedEnrollmentPresets = vi.fn();
const createEnrollmentPreset = vi.fn();
const updateEnrollmentPreset = vi.fn();
const deleteEnrollmentPreset = vi.fn();

vi.mock("../../../lib/api", () => ({
    issueInstallArtifact: (...args: unknown[]) => issueInstallArtifact(...args),
    listInstallPlatforms: () => listInstallPlatforms(),
    getServerInfo: () => getServerInfo(),
    listEnrollmentPresets: (...args: unknown[]) =>
        listEnrollmentPresets(...args),
    seedEnrollmentPresets: (...args: unknown[]) =>
        seedEnrollmentPresets(...args),
    createEnrollmentPreset: (...args: unknown[]) =>
        createEnrollmentPreset(...args),
    updateEnrollmentPreset: (...args: unknown[]) =>
        updateEnrollmentPreset(...args),
    deleteEnrollmentPreset: (...args: unknown[]) =>
        deleteEnrollmentPreset(...args),
}));

// The new baseline-plugins step uses the marketplace catalog as its
// data source. Stub it out so the wizard test doesn't need a real
// catalog response — empty list is fine, the step renders an empty
// state and Next still advances.
vi.mock("../../../lib/api/marketplace", () => ({
    searchPlugins: vi.fn().mockResolvedValue([]),
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
    listEnrollmentPresets.mockReset();
    seedEnrollmentPresets.mockReset();
    createEnrollmentPreset.mockReset();
    updateEnrollmentPreset.mockReset();
    deleteEnrollmentPreset.mockReset();
    listInstallPlatforms.mockResolvedValue({
        channel: "stable",
        platforms: [
            { os: "linux", arch: "amd64" },
            { os: "linux", arch: "arm64" },
        ],
    });
    getServerInfo.mockResolvedValue({ public_addr: "203.0.113.5:13337" });
    // Default: project starts with no presets so the wizard's
    // pick_preset step shows the empty-state. Tests that exercise the
    // populated picker override this. Seed is a no-op for now (the
    // seed step only fires on empty list and tests don't care about
    // the seeded rows themselves unless they assert on them).
    listEnrollmentPresets.mockResolvedValue([]);
    seedEnrollmentPresets.mockResolvedValue([]);
});

describe("<EnrollAgentWizard>", () => {
    it("stays closed without ?enroll=1", () => {
        render("/projects/test-project/hosts");
        expect(screen.queryByTestId("enroll-wizard")).not.toBeInTheDocument();
    });

    it("opens with ?enroll=1 and lands on the pick-preset step", async () => {
        render("/projects/test-project/hosts?enroll=1");
        expect(await screen.findByTestId("enroll-wizard")).toBeInTheDocument();
        // Step indicator marks `pick_preset` as the active step now.
        const indicator = await screen.findByTestId("enroll-wizard-steps");
        const active = indicator.querySelector('[data-step="pick_preset"]');
        expect(active?.getAttribute("data-active")).toBe("true");
    });

    it("walks through refined steps and submits with skipped OS/arch", async () => {
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

        // Skip past the pick-preset landing screen to land on the
        // legacy step-1 (Server endpoint).
        await screen.findByTestId("enroll-wizard-pick-preset");
        await user.click(await screen.findByTestId("enroll-wizard-start-blank"));

        const server = await screen.findByTestId("enroll-wizard-server");
        const endpoint = server.querySelector(
            'input[placeholder="203.0.113.5:13337"]',
        ) as HTMLInputElement;
        expect(endpoint.value).toBe("203.0.113.5:13337");

        await user.click(screen.getByTestId("enroll-wizard-next")); // server -> tls
        await screen.findByTestId("enroll-wizard-download-tls");
        await user.click(screen.getByTestId("enroll-wizard-next")); // tls -> os
        await screen.findByTestId("enroll-wizard-os");
        await user.click(screen.getByTestId("enroll-wizard-next")); // os -> arch
        await user.click(screen.getByTestId("enroll-wizard-next")); // arch -> ttl
        await screen.findByTestId("enroll-wizard-ttl");
        await user.click(screen.getByTestId("enroll-wizard-next")); // ttl -> pat uses
        await screen.findByTestId("enroll-wizard-pat-max-uses");
        await user.click(screen.getByTestId("enroll-wizard-next")); // pat uses -> auto approve
        await screen.findByTestId("enroll-wizard-auto-approve");
        await user.click(screen.getByTestId("enroll-wizard-next")); // auto approve -> baseline plugins
        await screen.findByTestId("enroll-wizard-baseline-plugins");
        await user.click(screen.getByTestId("enroll-wizard-next")); // baseline plugins -> description
        await screen.findByTestId("enroll-wizard-description");
        await user.click(screen.getByTestId("enroll-wizard-next")); // description -> review
        await screen.findByTestId("enroll-wizard-review");
        await user.click(screen.getByTestId("enroll-wizard-generate"));

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

        await screen.findByTestId("enroll-wizard-pick-preset");
        await user.click(await screen.findByTestId("enroll-wizard-start-blank"));
        await screen.findByTestId("enroll-wizard-server");
        for (let i = 0; i < 9; i++) await user.click(screen.getByTestId("enroll-wizard-next"));
        await screen.findByTestId("enroll-wizard-review");
        await user.click(screen.getByTestId("enroll-wizard-generate"));

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

        await screen.findByTestId("enroll-wizard-pick-preset");
        await user.click(await screen.findByTestId("enroll-wizard-start-blank"));
        await screen.findByTestId("enroll-wizard-server");
        for (let i = 0; i < 9; i++) await user.click(screen.getByTestId("enroll-wizard-next"));
        await screen.findByTestId("enroll-wizard-review");
        await user.click(screen.getByTestId("enroll-wizard-generate"));

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

        await screen.findByTestId("enroll-wizard-pick-preset");
        await user.click(await screen.findByTestId("enroll-wizard-start-blank"));
        await screen.findByTestId("enroll-wizard-server");
        for (let i = 0; i < 9; i++) await user.click(screen.getByTestId("enroll-wizard-next"));
        await screen.findByTestId("enroll-wizard-review");
        await user.click(screen.getByTestId("enroll-wizard-generate"));

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

    // Picking a saved preset on the landing screen should jump
    // straight to Review with the preset's values applied. This is
    // the "repeat operator" speedup: one click of "Use" replaces the
    // 11-step walk-through.
    it("applies a preset's values and jumps to review", async () => {
        const preset = {
            preset_id: "epr_linux",
            project_id: "p1",
            name: "linux-prod",
            target_os: "linux",
            target_arch: "amd64",
            ttl_seconds: 600,
            pat_max_uses: 1,
            auto_approve: false,
            skip_tls_verification: true,
            plugin_specs: [{ plugin_id: "sys-info" }],
            pat_description: "Linux fleet",
            is_seed: false,
            created_at: "2026-05-01T00:00:00Z",
            updated_at: "2026-05-01T00:00:00Z",
        };
        listEnrollmentPresets.mockResolvedValueOnce([preset]);
        issueInstallArtifact.mockResolvedValue({
            download_id: "dl_p",
            download_token: "dl_p.secret",
            expires_at: new Date(Date.now() + 60_000).toISOString(),
            server_endpoint: "203.0.113.5:13337",
            install_command: "curl-from-preset",
            bundle_url: "https://example.test/install/dl_p",
        });

        const user = userEvent.setup();
        render("/projects/test-project/hosts?enroll=1");

        await screen.findByTestId("enroll-wizard-preset-list");
        await user.click(
            screen.getByTestId("enroll-wizard-preset-pick-epr_linux"),
        );

        await screen.findByTestId("enroll-wizard-review");
        await user.click(screen.getByTestId("enroll-wizard-generate"));

        await waitFor(() =>
            expect(screen.getByTestId("enroll-wizard-run")).toBeInTheDocument(),
        );
        // The issued payload should reflect the preset, not the
        // wizard's blank defaults.
        const [, body] = issueInstallArtifact.mock.calls[0];
        expect(body.target_os).toBe("linux");
        expect(body.target_arch).toBe("amd64");
        expect(body.ttl_seconds).toBe(600);
        expect(body.pat_max_uses).toBe(1);
        // Wizard sends plugin_specs (rich shape) lifted directly
        // from preset.plugin_specs.
        expect(
            (body.plugin_specs ?? []).map(
                (s: { plugin_id: string }) => s.plugin_id,
            ),
        ).toEqual(["sys-info"]);
        expect(body.pat_description).toBe("Linux fleet");
        expect(body.auto_approve).toBe(false);
    });

    // Empty list on first open triggers the seed call. The picker
    // then renders whatever the server returns from /seed (which
    // INSERT-OR-IGNOREs the three system defaults).
    it("seeds system presets when the project list is empty", async () => {
        listEnrollmentPresets.mockResolvedValueOnce([]);
        seedEnrollmentPresets.mockResolvedValueOnce([
            {
                preset_id: "epr_linux_seed",
                project_id: "p1",
                name: "Linux x86_64",
                target_os: "linux",
                target_arch: "amd64",
                auto_approve: false,
                skip_tls_verification: true,
                is_seed: true,
                created_at: "2026-05-01T00:00:00Z",
                updated_at: "2026-05-01T00:00:00Z",
            },
        ]);

        render("/projects/test-project/hosts?enroll=1");

        await waitFor(() => {
            expect(seedEnrollmentPresets).toHaveBeenCalledTimes(1);
        });
        await screen.findByTestId(
            "enroll-wizard-preset-pick-epr_linux_seed",
        );
    });

    // Save-as-preset on the Review step calls createEnrollmentPreset
    // with the current wizard snapshot so operators can converge on
    // their own defaults without leaving the wizard.
    it("saves the current wizard state as a new preset", async () => {
        createEnrollmentPreset.mockResolvedValue({
            preset_id: "epr_new",
            project_id: "p1",
            name: "blank-default",
            auto_approve: false,
            skip_tls_verification: true,
            is_seed: false,
            created_at: "2026-05-01T00:00:00Z",
            updated_at: "2026-05-01T00:00:00Z",
        });

        const user = userEvent.setup();
        render("/projects/test-project/hosts?enroll=1");

        await screen.findByTestId("enroll-wizard-pick-preset");
        await user.click(screen.getByTestId("enroll-wizard-start-blank"));
        await screen.findByTestId("enroll-wizard-server");
        for (let i = 0; i < 9; i++)
            await user.click(screen.getByTestId("enroll-wizard-next"));
        await screen.findByTestId("enroll-wizard-review");

        await user.click(screen.getByTestId("enroll-wizard-save-preset"));
        const nameInput = await screen.findByTestId(
            "enroll-wizard-preset-name",
        );
        await user.type(nameInput, "blank-default");
        await user.click(
            screen.getByTestId("enroll-wizard-preset-save-confirm"),
        );

        await waitFor(() =>
            expect(createEnrollmentPreset).toHaveBeenCalledTimes(1),
        );
        const [pid, body] = createEnrollmentPreset.mock.calls[0];
        expect(pid).toBe("p1");
        expect(body.name).toBe("blank-default");
        expect(body.skip_tls_verification).toBe(true);
        // The wizard left the defaults blank (server endpoint
        // pre-fills from getServerInfo, so it's the only non-blank
        // field that round-trips).
        expect(body.server_endpoint).toBe("203.0.113.5:13337");
    });
});
