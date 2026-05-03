// Spec for the per-plugin marketplace detail page.
//
// This is the destination operator lands at when clicking a card on
// /marketplace. Coverage targets the data the page must surface:
//   - plugin name, version, description, author, license
//   - declared capabilities (rendered with risk metadata)
//   - all versions known to the catalog
//   - a "back to marketplace" affordance

import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../../lib/api/marketplace", () => ({
    pluginVersions: vi.fn(),
}));

import { pluginVersions } from "../../lib/api/marketplace";
import PluginDetailPage from "./PluginDetailPage";

const mocked = {
    pluginVersions: vi.mocked(pluginVersions),
};

beforeEach(() => {
    mocked.pluginVersions.mockReset();
});

function renderRoute(initialPath: string) {
    const client = new QueryClient({
        defaultOptions: {
            queries: { retry: false, refetchOnMount: false, refetchOnWindowFocus: false },
        },
    });
    function Wrapper({ children }: { children: ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }
    return render(
        <MemoryRouter initialEntries={[initialPath]}>
            <Routes>
                <Route path="/marketplace/plugins/:pluginID" element={<PluginDetailPage />} />
            </Routes>
        </MemoryRouter>,
        { wrapper: Wrapper },
    );
}

const versions = [
    {
        plugin_id: "com.example.x",
        version: "2.0.0",
        name: "Example Plugin",
        author: "Example Author",
        license: "Apache-2.0",
        homepage: "https://example.test",
        description: "Reads files for great justice.",
        latest_version: "2.0.0",
        publisher_key_id: "abc",
        wasm_url: "",
        signature_url: "",
        wasm_sha256_hex: "",
        capabilities: ["fs.read", "log"],
        fetched_at_unix: 1_700_000_000,
    },
    {
        plugin_id: "com.example.x",
        version: "1.0.0",
        name: "Example Plugin",
        author: "Example Author",
        license: "Apache-2.0",
        homepage: "https://example.test",
        description: "Reads files for great justice.",
        latest_version: "2.0.0",
        publisher_key_id: "abc",
        wasm_url: "",
        signature_url: "",
        wasm_sha256_hex: "",
        capabilities: ["fs.read"],
        fetched_at_unix: 1_690_000_000,
    },
];

describe("<PluginDetailPage>", () => {
    it("shows the plugin header and description", async () => {
        mocked.pluginVersions.mockResolvedValueOnce(versions);
        renderRoute("/marketplace/plugins/com.example.x");
        await waitFor(() => {
            expect(screen.getByText(/Example Plugin/i)).toBeInTheDocument();
        });
        expect(screen.getByText(/Reads files for great justice/i)).toBeInTheDocument();
        expect(screen.getByText(/Example Author/i)).toBeInTheDocument();
        expect(screen.getByText(/Apache-2\.0/i)).toBeInTheDocument();
    });

    it("renders declared capabilities with their human-readable labels", async () => {
        mocked.pluginVersions.mockResolvedValueOnce(versions);
        renderRoute("/marketplace/plugins/com.example.x");
        await waitFor(() => {
            // The latest version's capabilities — the page surfaces
            // the label from capabilityMeta, not the raw family string.
            expect(screen.getByText(/Filesystem read/i)).toBeInTheDocument();
            expect(screen.getByText(/Logging/i)).toBeInTheDocument();
        });
    });

    it("lists every version in the version table, marking the latest", async () => {
        mocked.pluginVersions.mockResolvedValueOnce(versions);
        renderRoute("/marketplace/plugins/com.example.x");
        await waitFor(() => {
            expect(screen.getByText("2.0.0")).toBeInTheDocument();
        });
        expect(screen.getByText("1.0.0")).toBeInTheDocument();
        // The latest pointer surfaces as a badge / inline marker —
        // match against text that distinguishes 2.0.0 from 1.0.0.
        const latestBadges = screen.getAllByText(/latest/i);
        expect(latestBadges.length).toBeGreaterThanOrEqual(1);
    });

    it("renders an empty state when the plugin id is unknown", async () => {
        mocked.pluginVersions.mockResolvedValueOnce([]);
        renderRoute("/marketplace/plugins/com.example.does-not-exist");
        await waitFor(() => {
            expect(screen.getByText(/not found/i)).toBeInTheDocument();
        });
    });
});
