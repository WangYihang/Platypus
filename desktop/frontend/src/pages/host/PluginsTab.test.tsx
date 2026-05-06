// Spec for the per-host PluginsTab.
//
// Coverage focus:
//   - empty state vs populated list
//   - row controls call the API client (toggle / uninstall / logs)
//   - uninstall is gated behind an explicit confirmation
//   - Available section: shows un-installed system plugins, filtered
//     by host OS, with Install action wired to installFromSystem
// We don't test the offline (404) error path here — it's handled
// upstream by the shared error boundary; reproducing it would require
// faking the auth wrapper.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const toastMocks = vi.hoisted(() => ({
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
}));
vi.mock("sonner", () => ({
    toast: toastMocks,
}));

vi.mock("../../lib/api/agents/plugins", () => ({
    listPlugins: vi.fn(),
    enablePlugin: vi.fn(),
    uninstallPlugin: vi.fn(),
    pluginLogs: vi.fn(),
    installFromSystem: vi.fn(),
}));

vi.mock("../../lib/api/system_plugins", () => ({
    listSystemPlugins: vi.fn(),
}));

import {
    listPlugins,
    enablePlugin,
    uninstallPlugin,
    pluginLogs,
    installFromSystem,
} from "../../lib/api/agents/plugins";
import { listSystemPlugins } from "../../lib/api/system_plugins";
import PluginsTab from "./PluginsTab";

function makeClient(): QueryClient {
    return new QueryClient({
        defaultOptions: {
            queries: { retry: false, refetchOnMount: false, refetchOnWindowFocus: false },
            mutations: { retry: false },
        },
    });
}

function renderTab(active = true, hostOS = "linux") {
    const client = makeClient();
    function Wrapper({ children }: { children: ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }
    return render(
        <PluginsTab
            projectID="proj1"
            hostID="agent-a1"
            agentID="agent-a1"
            hostOS={hostOS}
            active={active}
        />,
        { wrapper: Wrapper },
    );
}

const mocked = {
    listPlugins: vi.mocked(listPlugins),
    enablePlugin: vi.mocked(enablePlugin),
    uninstallPlugin: vi.mocked(uninstallPlugin),
    pluginLogs: vi.mocked(pluginLogs),
    installFromSystem: vi.mocked(installFromSystem),
    listSystemPlugins: vi.mocked(listSystemPlugins),
};

beforeEach(() => {
    // vi.mock binds these vi.fn() instances at module load; reset
    // their call history per-test so assertions like "did not call"
    // don't see leakage from earlier specs.
    mocked.listPlugins.mockReset();
    mocked.enablePlugin.mockReset();
    mocked.uninstallPlugin.mockReset();
    mocked.pluginLogs.mockReset();
    mocked.installFromSystem.mockReset();
    mocked.listSystemPlugins.mockReset();
    // Default: empty system catalog so the legacy specs don't have
    // to think about Available section state.
    mocked.listSystemPlugins.mockResolvedValue([]);
});

describe("<PluginsTab>", () => {
    it("renders the empty state when the agent has no plugins", async () => {
        mocked.listPlugins.mockResolvedValueOnce([]);
        renderTab();
        await waitFor(() => {
            expect(screen.getByText(/no plugins installed/i)).toBeInTheDocument();
        });
    });

    it("lists installed plugins and shows their granted capabilities", async () => {
        mocked.listPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                author: "Platypus",
                enabled: true,
                granted_capabilities: ["log", "fs.read"],
                install_unix: 1_700_000_000,
                publisher_key_id: "abcd",
            },
        ]);
        renderTab();
        await waitFor(() => {
            expect(screen.getByText(/System Info/i)).toBeInTheDocument();
        });
        // Family names come from granted_capabilities and we surface
        // them so the operator can see what's been authorised.
        expect(screen.getByText(/log/)).toBeInTheDocument();
        expect(screen.getByText(/fs\.read/)).toBeInTheDocument();
    });

    it("toggling enabled fires enablePlugin", async () => {
        mocked.listPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                author: "Platypus",
                enabled: true,
                granted_capabilities: [],
                install_unix: 0,
                publisher_key_id: "k",
            },
        ]);
        mocked.enablePlugin.mockResolvedValueOnce(undefined);
        // After mutation react-query will refetch; the second list
        // call returns the toggled state.
        mocked.listPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                author: "Platypus",
                enabled: false,
                granted_capabilities: [],
                install_unix: 0,
                publisher_key_id: "k",
            },
        ]);

        renderTab();
        await waitFor(() => screen.getByText(/System Info/i));

        // The Switch primitive renders as role="switch" with an
        // accessible name from its label.
        const toggle = screen.getByRole("switch", { name: /enabled/i });
        fireEvent.click(toggle);

        await waitFor(() => {
            expect(mocked.enablePlugin).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-info",
                false,
            );
        });
    });

    it("uninstall opens a confirm dialog and calls uninstallPlugin on accept", async () => {
        mocked.listPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                author: "Platypus",
                enabled: true,
                granted_capabilities: [],
                install_unix: 0,
                publisher_key_id: "k",
            },
        ]);
        mocked.uninstallPlugin.mockResolvedValueOnce(undefined);
        mocked.listPlugins.mockResolvedValueOnce([]);

        const user = userEvent.setup();
        renderTab();
        await waitFor(() => screen.getByText(/System Info/i));

        // Radix DropdownMenu renders into a portal on click; userEvent
        // drives the keyboard/pointer dance correctly where fireEvent
        // doesn't (the trigger toggles via onPointerDown which
        // fireEvent.click doesn't simulate).
        await user.click(screen.getByRole("button", { name: /more actions/i }));
        await user.click(await screen.findByRole("menuitem", { name: /uninstall/i }));

        // AlertDialog uses role="alertdialog" — match either.
        await waitFor(() => {
            const d =
                screen.queryByRole("alertdialog", { name: /uninstall/i }) ||
                screen.queryByRole("dialog", { name: /uninstall/i });
            expect(d).not.toBeNull();
        });
        await user.click(screen.getByRole("button", { name: /^uninstall$/i }));

        await waitFor(() => {
            expect(mocked.uninstallPlugin).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-info",
                expect.objectContaining({ purgeState: false }),
            );
        });
    });

    it("does not list plugins when the tab is inactive", () => {
        mocked.listPlugins.mockResolvedValue([]);
        renderTab(false);
        expect(mocked.listPlugins).not.toHaveBeenCalled();
    });

    // ---------------- Available section (M-phase follow-up) ----------------

    it("renders an Available section with un-installed system plugins", async () => {
        mocked.listPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                author: "Platypus",
                enabled: true,
                granted_capabilities: ["log", "fs.read"],
                install_unix: 0,
                publisher_key_id: "k",
            },
        ]);
        mocked.listSystemPlugins.mockResolvedValueOnce([
            {
                // already installed → must NOT appear in Available
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                capabilities: ["log", "fs.read"],
            },
            {
                id: "com.platypus.sys-procs-linux",
                name: "Process list (linux)",
                version: "2.0.0",
                capabilities: ["log", "fs.read"],
            },
            {
                id: "com.platypus.sys-disk-linux",
                name: "Disk usage (linux)",
                version: "1.0.0",
                capabilities: ["log", "exec"],
            },
        ]);

        renderTab();
        await waitFor(() => screen.getByRole("heading", { name: /available/i }));

        // sys-info is installed → only in Installed section, not Available.
        expect(screen.getByText(/Process list \(linux\)/)).toBeInTheDocument();
        expect(screen.getByText(/Disk usage \(linux\)/)).toBeInTheDocument();

        // Each Available row has an Install button.
        const installButtons = screen.getAllByRole("button", { name: /^install$/i });
        expect(installButtons.length).toBe(2);
    });

    it("filters Available by host OS via os_targets", async () => {
        mocked.listPlugins.mockResolvedValueOnce([]);
        mocked.listSystemPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-procs-linux",
                name: "Process list (linux)",
                version: "2.0.0",
                capabilities: ["fs.read"],
                os_targets: ["linux"],
            },
            {
                id: "com.platypus.sys-procs-darwin",
                name: "Process list (darwin)",
                version: "1.0.0",
                capabilities: ["exec"],
                os_targets: ["darwin"],
            },
            {
                id: "com.platypus.sys-info",
                name: "System Info (multi)",
                version: "2.0.0",
                capabilities: ["log"],
                // No os_targets → applies everywhere
            },
        ]);

        renderTab(true, "linux");
        await waitFor(() => screen.getByText(/Process list \(linux\)/));

        // darwin variant must be hidden on a linux host.
        expect(screen.queryByText(/Process list \(darwin\)/)).toBeNull();
        // No-os_targets entry shows up.
        expect(screen.getByText(/System Info \(multi\)/)).toBeInTheDocument();
    });

    it("clicking Install on an Available row calls installFromSystem", async () => {
        mocked.listPlugins.mockResolvedValueOnce([]);
        mocked.listSystemPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-procs-linux",
                name: "Process list (linux)",
                version: "2.0.0",
                capabilities: ["log", "fs.read"],
            },
        ]);
        mocked.installFromSystem.mockResolvedValueOnce({
            // The InstallResult shape carries more fields; the
            // PluginsTab only consumes "ok"-vs-error and refetches
            // installed plugins on success.
            plugin_id: "com.platypus.sys-procs-linux",
            version: "2.0.0",
            progress: [],
        } as unknown as Awaited<ReturnType<typeof installFromSystem>>);
        // Refetch returns the now-installed plugin.
        mocked.listPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-procs-linux",
                name: "Process list (linux)",
                version: "2.0.0",
                author: "Platypus",
                enabled: true,
                granted_capabilities: ["log", "fs.read"],
                install_unix: 0,
                publisher_key_id: "k",
            },
        ]);

        const user = userEvent.setup();
        renderTab();
        await waitFor(() => screen.getByText(/Process list \(linux\)/));

        await user.click(screen.getByRole("button", { name: /^install$/i }));

        await waitFor(() => {
            expect(mocked.installFromSystem).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                expect.objectContaining({
                    pluginID: "com.platypus.sys-procs-linux",
                    version: "2.0.0",
                    grantedCapabilities: ["log", "fs.read"],
                }),
            );
        });
    });

    it("hides the Available section when every system plugin is already installed", async () => {
        mocked.listPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                author: "Platypus",
                enabled: true,
                granted_capabilities: ["log"],
                install_unix: 0,
                publisher_key_id: "k",
            },
        ]);
        mocked.listSystemPlugins.mockResolvedValueOnce([
            {
                id: "com.platypus.sys-info",
                name: "System Info",
                version: "2.0.0",
                capabilities: ["log"],
            },
        ]);

        renderTab();
        await waitFor(() => screen.getByText(/System Info/));

        expect(screen.queryByRole("heading", { name: /available/i })).toBeNull();
    });
});
