// Spec for the per-host PluginsTab.
//
// Coverage focus:
//   - empty state vs populated list
//   - row controls call the API client (toggle / uninstall / logs)
//   - uninstall is gated behind an explicit confirmation
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
}));

import {
    listPlugins,
    enablePlugin,
    uninstallPlugin,
    pluginLogs,
} from "../../lib/api/agents/plugins";
import PluginsTab from "./PluginsTab";

function makeClient(): QueryClient {
    return new QueryClient({
        defaultOptions: {
            queries: { retry: false, refetchOnMount: false, refetchOnWindowFocus: false },
            mutations: { retry: false },
        },
    });
}

function renderTab(active = true) {
    const client = makeClient();
    function Wrapper({ children }: { children: ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }
    return render(
        <PluginsTab
            projectID="proj1"
            hostID="agent-a1"
            agentID="agent-a1"
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
};

beforeEach(() => {
    // vi.mock binds these vi.fn() instances at module load; reset
    // their call history per-test so assertions like "did not call"
    // don't see leakage from earlier specs.
    mocked.listPlugins.mockReset();
    mocked.enablePlugin.mockReset();
    mocked.uninstallPlugin.mockReset();
    mocked.pluginLogs.mockReset();
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
});
