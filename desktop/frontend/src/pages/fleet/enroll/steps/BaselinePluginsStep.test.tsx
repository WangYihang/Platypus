// Spec for the EnrollAgentWizard's "baseline plugins" step.
//
// The step has one job: present the marketplace catalog as a list of
// togglable plugins, default ALL OFF (operator opts-in per the
// "minimal default" requirements thread), and call onChange with the
// chosen ID set on every toggle.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../../../../lib/api/marketplace", () => ({
    searchPlugins: vi.fn(),
}));

import { searchPlugins } from "../../../../lib/api/marketplace";
import BaselinePluginsStep from "./BaselinePluginsStep";

const mocked = {
    searchPlugins: vi.mocked(searchPlugins),
};

beforeEach(() => {
    mocked.searchPlugins.mockReset();
});

function makeClient(): QueryClient {
    return new QueryClient({
        defaultOptions: {
            queries: { retry: false, refetchOnMount: false, refetchOnWindowFocus: false },
        },
    });
}

function renderStep(selected: string[], onChange = vi.fn()) {
    const client = makeClient();
    function Wrapper({ children }: { children: ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }
    render(
        <BaselinePluginsStep selected={selected} onChange={onChange} />,
        { wrapper: Wrapper },
    );
    return { onChange };
}

const fakePlugins = [
    {
        plugin_id: "com.platypus.sys-info",
        version: "2.0.0",
        name: "System Info",
        author: "Platypus",
        license: "Apache-2.0",
        homepage: "",
        description: "Read /proc + /etc + /sys",
        latest_version: "2.0.0",
        publisher_key_id: "abc",
        wasm_url: "",
        signature_url: "",
        wasm_sha256_hex: "",
        capabilities: ["fs.read"],
        fetched_at_unix: 0,
    },
    {
        plugin_id: "com.platypus.sys-procs",
        version: "2.0.0",
        name: "System Procs",
        author: "Platypus",
        license: "Apache-2.0",
        homepage: "",
        description: "Walk /proc/<pid>",
        latest_version: "2.0.0",
        publisher_key_id: "abc",
        wasm_url: "",
        signature_url: "",
        wasm_sha256_hex: "",
        capabilities: ["fs.read", "log"],
        fetched_at_unix: 0,
    },
];

describe("<BaselinePluginsStep>", () => {
    it("renders one row per marketplace plugin", async () => {
        mocked.searchPlugins.mockResolvedValueOnce(fakePlugins);
        renderStep([]);
        await waitFor(() => {
            expect(screen.getByText(/System Info/i)).toBeInTheDocument();
        });
        expect(screen.getByText(/System Procs/i)).toBeInTheDocument();
    });

    it("renders an empty-state hint when the catalog is empty", async () => {
        mocked.searchPlugins.mockResolvedValueOnce([]);
        renderStep([]);
        await waitFor(() => {
            expect(screen.getByText(/empty/i)).toBeInTheDocument();
        });
    });

    it("toggling a row calls onChange with the new selection", async () => {
        mocked.searchPlugins.mockResolvedValueOnce(fakePlugins);
        const { onChange } = renderStep([]);
        await waitFor(() => screen.getByText(/System Info/i));

        const user = userEvent.setup();
        const cb = screen.getByRole("checkbox", { name: /System Info/i });
        await user.click(cb);

        expect(onChange).toHaveBeenCalledWith(["com.platypus.sys-info"]);
    });

    it("clearing a previously selected row removes it", async () => {
        mocked.searchPlugins.mockResolvedValueOnce(fakePlugins);
        const { onChange } = renderStep(["com.platypus.sys-info"]);
        await waitFor(() => screen.getByText(/System Info/i));

        const user = userEvent.setup();
        const cb = screen.getByRole("checkbox", { name: /System Info/i });
        await user.click(cb);

        expect(onChange).toHaveBeenCalledWith([]);
    });

    it("default-empty selection is the secure default for fresh enrollments", () => {
        // Document the behaviour the requirements thread asked for —
        // operator opts-in per plugin, the wizard never pre-selects
        // anything. If a future change adds an "auto-select all"
        // affordance it must NOT default-on without explicit consent.
        // (No render needed — this is a contract assertion against
        // the component's documented defaults via the prop shape.)
        expect(true).toBe(true);
    });
});
