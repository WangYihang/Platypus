// Spec for the "Install from Marketplace" dialog operating from the
// per-host PluginsTab. The dialog has three phases:
//
//   1. PICK     — operator searches/picks a plugin from the catalog
//   2. CONFIRM  — operator ticks the capabilities to grant
//   3. PROGRESS — server streams install phases; ends in installed/
//                 failed
//
// We mock the marketplace + per-agent API clients so the spec runs
// without a backend. Coverage is the path through the three phases
// + the "no plugins in catalog" empty state.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const toastMocks = vi.hoisted(() => ({
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: toastMocks }));

vi.mock("../../../lib/api/marketplace", () => ({
    searchPlugins: vi.fn(),
}));
vi.mock("../../../lib/api/agents/plugins", () => ({
    installFromMarketplace: vi.fn(),
}));

import { searchPlugins } from "../../../lib/api/marketplace";
import { installFromMarketplace } from "../../../lib/api/agents/plugins";
import InstallFromMarketplaceDialog from "./InstallFromMarketplaceDialog";

const mocked = {
    searchPlugins: vi.mocked(searchPlugins),
    installFromMarketplace: vi.mocked(installFromMarketplace),
};

beforeEach(() => {
    mocked.searchPlugins.mockReset();
    mocked.installFromMarketplace.mockReset();
});

function makeClient(): QueryClient {
    return new QueryClient({
        defaultOptions: {
            queries: { retry: false, refetchOnMount: false, refetchOnWindowFocus: false },
            mutations: { retry: false },
        },
    });
}

function renderOpen() {
    const onClose = vi.fn();
    const onInstalled = vi.fn();
    const client = makeClient();
    function Wrapper({ children }: { children: ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }
    render(
        <InstallFromMarketplaceDialog
            open={true}
            projectID="proj1"
            agentID="agent-a1"
            onClose={onClose}
            onInstalled={onInstalled}
        />,
        { wrapper: Wrapper },
    );
    return { onClose, onInstalled };
}

const fakePlugin = {
    plugin_id: "com.example.x",
    version: "1.0.0",
    name: "Example Plugin",
    author: "Example Author",
    license: "Apache-2.0",
    homepage: "",
    description: "Does X",
    latest_version: "1.0.0",
    publisher_key_id: "abc",
    wasm_url: "",
    signature_url: "",
    wasm_sha256_hex: "",
    capabilities: ["fs.read", "log"],
    fetched_at_unix: 0,
};

describe("<InstallFromMarketplaceDialog>", () => {
    it("shows the catalog list when opened", async () => {
        mocked.searchPlugins.mockResolvedValueOnce([fakePlugin]);
        renderOpen();
        await waitFor(() => {
            expect(screen.getByText(/Example Plugin/i)).toBeInTheDocument();
        });
    });

    it("shows an empty state when the catalog is empty", async () => {
        mocked.searchPlugins.mockResolvedValueOnce([]);
        renderOpen();
        await waitFor(() => {
            expect(screen.getByText(/no plugins/i)).toBeInTheDocument();
        });
    });

    it("picks a plugin → opens cap confirm → install fires", async () => {
        mocked.searchPlugins.mockResolvedValueOnce([fakePlugin]);
        mocked.installFromMarketplace.mockResolvedValueOnce({
            status: "installed",
            plugin_id: fakePlugin.plugin_id,
            version: fakePlugin.version,
            progress: [
                { phase: "PHASE_VERIFY_SIG" },
                { phase: "PHASE_INSTALLED" },
            ],
        });

        const { onInstalled } = renderOpen();
        const user = userEvent.setup();

        await waitFor(() => screen.getByText(/Example Plugin/i));

        // Pick the plugin (its row's name is clickable).
        await user.click(screen.getByText(/Example Plugin/i));

        // CapabilityConfirmDialog opens with the plugin's declared
        // capabilities. The new UX leads with collection presets;
        // the plugin declares fs.read + log, so picking "Read-only
        // inspection" grants exactly { fs.read, log } (sysinfo / kv
        // are filtered because the plugin doesn't declare them).
        await waitFor(() => {
            expect(
                screen.getByRole("button", { name: /read-only inspection/i }),
            ).toBeInTheDocument();
        });
        await user.click(screen.getByRole("button", { name: /read-only inspection/i }));
        await user.click(screen.getByRole("button", { name: /^install$/i }));

        await waitFor(() => {
            expect(mocked.installFromMarketplace).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                expect.objectContaining({
                    pluginID: "com.example.x",
                    version: "1.0.0",
                    grantedCapabilities: expect.arrayContaining(["log", "fs.read"]),
                }),
            );
        });

        // After install completes the parent is notified.
        await waitFor(() => {
            expect(onInstalled).toHaveBeenCalled();
        });
    });

    it("renders a failure when the install endpoint returns failed", async () => {
        mocked.searchPlugins.mockResolvedValueOnce([fakePlugin]);
        mocked.installFromMarketplace.mockResolvedValueOnce({
            status: "failed",
            plugin_id: fakePlugin.plugin_id,
            version: fakePlugin.version,
            progress: [
                {
                    phase: "PHASE_FAILED",
                    error_code: "verify_sig",
                    error_message: "bad signature",
                },
            ],
        });

        renderOpen();
        const user = userEvent.setup();
        await waitFor(() => screen.getByText(/Example Plugin/i));
        await user.click(screen.getByText(/Example Plugin/i));
        // Just open + immediately install (empty grant set is allowed).
        await waitFor(() =>
            screen.getByRole("button", { name: /read-only inspection/i }),
        );
        await user.click(screen.getByRole("button", { name: /^install$/i }));

        await waitFor(() => {
            // The failure message + code surface in the progress view.
            expect(screen.getByText(/bad signature/i)).toBeInTheDocument();
        });
    });
});
