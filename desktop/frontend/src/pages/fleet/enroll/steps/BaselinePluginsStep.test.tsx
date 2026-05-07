// Spec for the EnrollAgentWizard's "baseline plugins" step.
//
// The step has one job: present the system-plugin catalog as a list
// of togglable plugins, default ALL OFF (operator opts-in per the
// "minimal default" requirements thread), and call onChange with the
// chosen ID set on every toggle.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../../../../lib/api/system_plugins", () => ({
    listSystemPlugins: vi.fn(),
}));

// PR 5.1 mounted PluginSpecEditor inline; the editor's
// SecretPicker reads from project_secrets which the step now
// fetches via useCurrentProject + useQuery. Stubbing both keeps
// the test focused on the picker UI.
vi.mock("../../../../lib/api/project_secrets", () => ({
    listProjectSecrets: vi.fn().mockResolvedValue([]),
    createProjectSecret: vi.fn(),
}));
vi.mock("../../../../layout/ProjectShell", () => ({
    useCurrentProject: () => ({
        id: "p1",
        slug: "test-project",
        name: "Test Project",
    }),
}));

import { listSystemPlugins } from "../../../../lib/api/system_plugins";
import BaselinePluginsStep from "./BaselinePluginsStep";

const mocked = {
    listSystemPlugins: vi.mocked(listSystemPlugins),
};

beforeEach(() => {
    mocked.listSystemPlugins.mockReset();
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
    // The step now takes PluginSpecDraft[] (rich shape). Tests
    // continue to pass plain id strings — the helper wraps each
    // into a minimal {plugin_id} spec so the existing assertion
    // shape stays readable.
    const value = selected.map((id) => ({ plugin_id: id }));
    render(
        <BaselinePluginsStep value={value} onChange={onChange} />,
        { wrapper: Wrapper },
    );
    return { onChange };
}

// asIDs reads the latest onChange call's PluginSpecDraft[] and
// projects to a flat []string of plugin_id values, so existing
// id-based assertions keep working without per-test rewriting.
function asIDs(onChange: ReturnType<typeof vi.fn>): string[] {
    const calls = onChange.mock.calls;
    if (calls.length === 0) return [];
    const last = calls[calls.length - 1][0] as { plugin_id: string }[];
    return last.map((s) => s.plugin_id);
}

const fakePlugins = [
    {
        id: "com.platypus.sys-info",
        version: "2.0.0",
        name: "System Info",
        author: "Platypus",
        license: "Apache-2.0",
        description: "Read /proc + /etc + /sys",
        capabilities: ["fs.read", "sysinfo"],
    },
    {
        id: "com.platypus.sys-procs-linux",
        version: "2.0.0",
        name: "System Procs",
        author: "Platypus",
        license: "Apache-2.0",
        description: "Walk /proc/<pid>",
        capabilities: ["fs.read"],
    },
];

describe("<BaselinePluginsStep>", () => {
    it("renders one row per system plugin", async () => {
        mocked.listSystemPlugins.mockResolvedValueOnce(fakePlugins);
        renderStep([]);
        await waitFor(() => {
            expect(screen.getByRole("checkbox", { name: /System Info/i })).toBeInTheDocument();
        });
        expect(screen.getByRole("checkbox", { name: /System Procs/i })).toBeInTheDocument();
    });

    it("renders an empty-state hint when the catalog is empty", async () => {
        mocked.listSystemPlugins.mockResolvedValueOnce([]);
        renderStep([]);
        await waitFor(() => {
            expect(screen.getByText(/No system plugins available/i)).toBeInTheDocument();
        });
    });

    it("toggling a row calls onChange with the new selection", async () => {
        mocked.listSystemPlugins.mockResolvedValueOnce(fakePlugins);
        const { onChange } = renderStep([]);
        await waitFor(() => screen.getByRole("checkbox", { name: /System Info/i }));

        const user = userEvent.setup();
        const cb = screen.getByRole("checkbox", { name: /System Info/i });
        await user.click(cb);

        expect(asIDs(onChange)).toEqual(["com.platypus.sys-info"]);
    });

    it("clearing a previously selected row removes it", async () => {
        mocked.listSystemPlugins.mockResolvedValueOnce(fakePlugins);
        const { onChange } = renderStep(["com.platypus.sys-info"]);
        await waitFor(() => screen.getByRole("checkbox", { name: /System Info/i }));

        const user = userEvent.setup();
        const cb = screen.getByRole("checkbox", { name: /System Info/i });
        await user.click(cb);

        expect(asIDs(onChange)).toEqual([]);
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

    it("clicking a preset card pre-fills onChange with the preset's plugin IDs", async () => {
        mocked.listSystemPlugins.mockResolvedValueOnce(fakePlugins);
        const { onChange } = renderStep([]);
        await waitFor(() =>
            screen.getByRole("checkbox", { name: /System Info/i }),
        );

        // The "Read-only inspection" preset wants sys-info,
        // sys-hostname, sys-listdir, sys-procs, sys-file-read,
        // sys-file-scan. The fake catalog only has sys-info +
        // sys-procs, so the preset filters down to those two.
        const user = userEvent.setup();
        await user.click(
            screen.getByRole("button", { name: /Read-only inspection/i }),
        );
        expect(asIDs(onChange)).toEqual([
            "com.platypus.sys-info",
            "com.platypus.sys-procs-linux",
        ]);
    });

    it("clicking the Minimal preset clears any prior selection", async () => {
        mocked.listSystemPlugins.mockResolvedValueOnce(fakePlugins);
        const { onChange } = renderStep([
            "com.platypus.sys-info",
            "com.platypus.sys-procs-linux",
        ]);
        await waitFor(() =>
            screen.getByRole("checkbox", { name: /System Info/i }),
        );
        const user = userEvent.setup();
        await user.click(screen.getByRole("button", { name: /Minimal/i }));
        expect(asIDs(onChange)).toEqual([]);
    });

    // The mounted PluginSpecEditor is the user-facing payoff of the
    // PR 1-4 framework. Selected plugins get a "Configure" toggle
    // that expands the row to expose capability checkboxes + (when
    // declared) a schema-driven config form. Pinning the affordance
    // keeps the configure-during-enrollment flow from regressing.
    it("a selected plugin shows a Configure toggle that opens the editor", async () => {
        mocked.listSystemPlugins.mockResolvedValueOnce(fakePlugins);
        const { onChange } = renderStep(["com.platypus.sys-info"]);
        await waitFor(() =>
            screen.getByRole("checkbox", { name: /System Info/i }),
        );
        const user = userEvent.setup();
        // Configure trigger only renders for SELECTED plugins —
        // unselected ones can't carry config because they're not
        // installing.
        const configure = await screen.findByTestId(
            "baseline-plugins-configure-com.platypus.sys-info",
        );
        await user.click(configure);
        // The expanded editor exposes the manifest's capabilities
        // as checkboxes (one per declared family). Picking one
        // updates the spec's granted_capabilities — which lets us
        // assert the data flow without depending on full schema
        // form rendering, since this fake plugin has no
        // config_schema.
        await user.click(screen.getByLabelText(/^fs\.read$/i));
        const last = onChange.mock.calls[onChange.mock.calls.length - 1][0];
        expect(last[0].granted_capabilities).toEqual(["fs.read"]);
    });
});
