// Spec for the <RPCTable> shared primitive.
//
// Coverage target:
//   - rendering rows from a mocked invokePluginRPC response
//   - column.render (custom cells) takes precedence over default field rendering
//   - request_form changes trigger refetch with the merged request body
//   - row-action confirm dialog → on accept → fires invokePluginRPC
//     with the action's method + args(row)
//   - danger-action gets visually distinct (red) styling
//   - refresh interval refetches at refreshMs
//   - error / empty / loading states render correctly
//
// We mock invokePluginRPC at the module boundary so the underlying
// fetch / base64 / bulk-endpoint plumbing isn't tested here (covered
// elsewhere via the bulk handler's own integration tests).

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
vi.mock("sonner", () => ({ toast: toastMocks }));

vi.mock("../../../../lib/api/agents/plugins", () => ({
    invokePluginRPC: vi.fn(),
}));

import { invokePluginRPC } from "../../../../lib/api/agents/plugins";
import RPCTable from "./RPCTable";

interface ListUnitsResponse {
    units: Array<{
        name: string;
        load: string;
        active: string;
        sub: string;
        description: string;
    }>;
}

function makeClient(): QueryClient {
    return new QueryClient({
        defaultOptions: {
            queries: { retry: false, refetchOnMount: false, refetchOnWindowFocus: false },
            mutations: { retry: false },
        },
    });
}

function Wrapper({ children }: { children: ReactNode }) {
    return (
        <QueryClientProvider client={makeClient()}>{children}</QueryClientProvider>
    );
}

const mocked = {
    invokePluginRPC: vi.mocked(invokePluginRPC),
};

beforeEach(() => {
    mocked.invokePluginRPC.mockReset();
    toastMocks.success.mockReset();
    toastMocks.error.mockReset();
    // The MetaStrip persists the operator's interval choice to
    // localStorage; clear it between tests so a previous test's
    // pick can't bleed into the next mount.
    try {
        window.localStorage.clear();
    } catch {
        // jsdom may reject under odd conditions; not fatal.
    }
});

const stubUnits: ListUnitsResponse = {
    units: [
        { name: "ssh.service", load: "loaded", active: "active", sub: "running", description: "OpenSSH server" },
        { name: "nginx.service", load: "loaded", active: "active", sub: "running", description: "nginx web server" },
        { name: "broken.service", load: "loaded", active: "failed", sub: "dead", description: "Broken thing" },
    ],
};

describe("<RPCTable>", () => {
    it("renders rows from the RPC response", async () => {
        mocked.invokePluginRPC.mockResolvedValueOnce(stubUnits);

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[
                        { field: "name", label: "Service", primary: true },
                        { field: "active", label: "State" },
                        { field: "description", label: "Description" },
                    ]}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));
        expect(screen.getByText("nginx.service")).toBeInTheDocument();
        expect(screen.getByText("broken.service")).toBeInTheDocument();
        expect(screen.getByText("OpenSSH server")).toBeInTheDocument();

        // Default headers from column.label
        expect(screen.getByRole("columnheader", { name: /service/i })).toBeInTheDocument();
        expect(screen.getByRole("columnheader", { name: /^state$/i })).toBeInTheDocument();
    });

    it("uses column.render when supplied (custom badge / styled cell)", async () => {
        mocked.invokePluginRPC.mockResolvedValueOnce(stubUnits);

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[
                        { field: "name", label: "Service", primary: true },
                        {
                            field: "active",
                            label: "State",
                            render: (row) => (
                                <span data-testid={`badge-${row.name}`}>
                                    {row.active.toUpperCase()}
                                </span>
                            ),
                        },
                    ]}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByTestId("badge-ssh.service"));
        expect(screen.getByTestId("badge-ssh.service")).toHaveTextContent("ACTIVE");
        expect(screen.getByTestId("badge-broken.service")).toHaveTextContent(
            "FAILED",
        );
    });

    it("buildRequest is called with form values; form changes refetch", async () => {
        mocked.invokePluginRPC.mockImplementation(async () => stubUnits);

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    requestForm={[
                        {
                            field: "state",
                            kind: "select",
                            label: "State",
                            options: [
                                { value: "", label: "All" },
                                { value: "active", label: "Active" },
                                { value: "failed", label: "Failed" },
                            ],
                            default: "",
                        },
                    ]}
                    buildRequest={(form) => ({
                        unit_type: "service",
                        ...(form.state ? { state: form.state } : {}),
                    })}
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));

        // Initial call uses the default form value (state="").
        expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
            "proj1",
            "agent-a1",
            "com.platypus.sys-systemd-linux",
            "list_units",
            { unit_type: "service" },
            expect.anything(),
        );

        // Operator picks "failed" — refetch with the new request.
        const select = screen.getByRole("combobox", { name: /state/i });
        await user.selectOptions(select, "failed");

        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-systemd-linux",
                "list_units",
                { unit_type: "service", state: "failed" },
                expect.anything(),
            );
        });
    });

    it("row-action with confirm dialog: accept fires invokePluginRPC with action method + args(row)", async () => {
        mocked.invokePluginRPC.mockResolvedValue(stubUnits);

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    actions={[
                        {
                            id: "restart",
                            label: "Restart",
                            method: "unit_action",
                            args: (row) => ({ name: row.name, action: "restart" }),
                            confirm: (row) => `Restart ${row.name}?`,
                        },
                    ]}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));

        // Open the action menu for ssh.service.
        await user.click(
            screen.getByRole("button", {
                name: /actions for ssh\.service/i,
            }),
        );
        await user.click(
            await screen.findByRole("menuitem", { name: /restart/i }),
        );

        // Confirm dialog appears; click confirm.
        await waitFor(() => {
            expect(screen.getByText(/restart ssh\.service\?/i)).toBeInTheDocument();
        });
        await user.click(
            screen.getByRole("button", { name: /^restart$/i }),
        );

        // Row-actions don't pass an AbortSignal (they're not query-
        // cancelable); the call site uses 5 positional args while the
        // list-RPC call uses 6 (with { signal }).
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-systemd-linux",
                "unit_action",
                { name: "ssh.service", action: "restart" },
            );
        });
    });

    it("danger-action gets red destructive styling on the menu item", async () => {
        mocked.invokePluginRPC.mockResolvedValueOnce(stubUnits);

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    actions={[
                        {
                            id: "stop",
                            label: "Stop",
                            method: "unit_action",
                            args: (row) => ({ name: row.name, action: "stop" }),
                            danger: true,
                        },
                    ]}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));
        await user.click(
            screen.getByRole("button", { name: /actions for ssh\.service/i }),
        );

        const item = await screen.findByRole("menuitem", { name: /stop/i });
        // Radix DropdownMenuItem applies the destructive style via
        // data-variant="destructive" when `variant=destructive` prop
        // is set; we test by querying for that attribute rather than
        // matching exact CSS classes (which churn).
        expect(item.getAttribute("data-variant")).toBe("destructive");
    });

    it("renders an empty state when rowsFrom returns []", async () => {
        mocked.invokePluginRPC.mockResolvedValueOnce({ units: [] });

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    emptyText="No services."
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText(/no services\./i));
    });

    it("surfaces errors from invokePluginRPC", async () => {
        mocked.invokePluginRPC.mockRejectedValueOnce(
            new Error("agent_offline: agent-a1"),
        );

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                />
            </Wrapper>,
        );

        await waitFor(() => {
            expect(screen.getByText(/agent_offline/i)).toBeInTheDocument();
        });
    });

    it("does not call invokePluginRPC when active=false", () => {
        mocked.invokePluginRPC.mockResolvedValue(stubUnits);

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    active={false}
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                />
            </Wrapper>,
        );

        expect(mocked.invokePluginRPC).not.toHaveBeenCalled();
    });

    // -----------------------------------------------------------------------
    // MetaStrip — last-updated indicator + manual refresh + interval picker
    // -----------------------------------------------------------------------

    it("MetaStrip: renders 'Updated …' once a query result lands", async () => {
        mocked.invokePluginRPC.mockResolvedValueOnce(stubUnits);

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                />
            </Wrapper>,
        );

        // After the first successful fetch, dataUpdatedAt is set;
        // MetaStrip renders "Updated just now" (since the test runs
        // < 5s after the mock resolution).
        await waitFor(() => screen.getByText("ssh.service"));
        await waitFor(() => {
            expect(screen.getByText(/^Updated /)).toBeInTheDocument();
        });
    });

    it("MetaStrip: clicking the refresh button triggers a refetch", async () => {
        mocked.invokePluginRPC.mockResolvedValue(stubUnits);

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));
        const callsBefore = mocked.invokePluginRPC.mock.calls.length;

        await user.click(
            screen.getByRole("button", { name: /refresh now/i }),
        );

        await waitFor(() => {
            expect(mocked.invokePluginRPC.mock.calls.length).toBeGreaterThan(
                callsBefore,
            );
        });
    });

    it("MetaStrip: changing the interval persists the choice to localStorage", async () => {
        mocked.invokePluginRPC.mockResolvedValue(stubUnits);

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    refreshMs={30000}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));

        await user.selectOptions(
            screen.getByRole("combobox", {
                name: /auto refresh interval/i,
            }),
            "5000",
        );

        // Persistence key matches the readPersistedInterval helper:
        // `rpc-refresh:<pluginID>:<agentID>`.
        expect(
            window.localStorage.getItem(
                "rpc-refresh:com.platypus.sys-systemd-linux:agent-a1",
            ),
        ).toBe("5000");
    });

    it("MetaStrip: persisted interval pre-fills the selector on next mount", async () => {
        mocked.invokePluginRPC.mockResolvedValue(stubUnits);

        // Pretend a prior session picked 10s for this (host, plugin).
        window.localStorage.setItem(
            "rpc-refresh:com.platypus.sys-systemd-linux:agent-a1",
            "10000",
        );

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    // Per-tab default is 30s but the persisted 10s
                    // should win.
                    refreshMs={30000}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));
        const select = screen.getByRole("combobox", {
            name: /auto refresh interval/i,
        }) as HTMLSelectElement;
        expect(select.value).toBe("10000");
    });

    // -----------------------------------------------------------------------
    // Pagination — offset/limit (sys-net, sys-services, sys-firewall, ...)
    // -----------------------------------------------------------------------

    it("offset pagination: injects offset+limit into the request and renders the footer", async () => {
        mocked.invokePluginRPC.mockResolvedValue({
            ...stubUnits,
            totalCount: 312,
            hasMore: true,
        });

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    pagination={{ kind: "offset", pageSize: 50 }}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));

        // Initial call carries offset=0 + limit=50 — the operator
        // didn't have to wire those into buildRequest manually.
        expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
            "proj1",
            "agent-a1",
            "com.platypus.sys-systemd-linux",
            "list_units",
            { offset: 0, limit: 50 },
            expect.anything(),
        );

        // Footer summary uses totalCount from the response.
        expect(screen.getByText(/showing 1.*3 of 312/i)).toBeInTheDocument();
    });

    it("offset pagination: Next button advances offset; Prev returns to offset 0", async () => {
        mocked.invokePluginRPC.mockResolvedValue({
            ...stubUnits,
            totalCount: 312,
            hasMore: true,
        });

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    pagination={{ kind: "offset", pageSize: 50 }}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));

        // Prev disabled at offset 0; Next enabled (hasMore=true).
        const prev = screen.getByRole("button", { name: /previous page/i });
        const next = screen.getByRole("button", { name: /next page/i });
        expect(prev).toBeDisabled();
        expect(next).toBeEnabled();

        await user.click(next);
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-systemd-linux",
                "list_units",
                { offset: 50, limit: 50 },
                expect.anything(),
            );
        });

        // Now Prev is enabled.
        await waitFor(() => {
            expect(
                screen.getByRole("button", { name: /previous page/i }),
            ).toBeEnabled();
        });

        await user.click(screen.getByRole("button", { name: /previous page/i }));
        await waitFor(() => {
            const calls = mocked.invokePluginRPC.mock.calls.filter(
                (c) =>
                    c[3] === "list_units" &&
                    JSON.stringify(c[4]) === JSON.stringify({ offset: 0, limit: 50 }),
            );
            // First load + post-Prev = at least 2.
            expect(calls.length).toBeGreaterThanOrEqual(2);
        });
    });

    it("offset pagination: Next is disabled when hasMore is false", async () => {
        mocked.invokePluginRPC.mockResolvedValue({
            ...stubUnits,
            totalCount: 3,
            hasMore: false,
        });

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    pagination={{ kind: "offset", pageSize: 50 }}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));
        expect(screen.getByRole("button", { name: /next page/i })).toBeDisabled();
        expect(screen.getByText(/showing 1.*3 of 3/i)).toBeInTheDocument();
    });

    it("offset pagination: changing the page-size selector resets offset to 0 and refetches", async () => {
        mocked.invokePluginRPC.mockResolvedValue({
            ...stubUnits,
            totalCount: 312,
            hasMore: true,
        });

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    pagination={{
                        kind: "offset",
                        pageSize: 50,
                        pageSizeOptions: [25, 50, 100],
                    }}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));

        // Walk to page 2 first.
        await user.click(screen.getByRole("button", { name: /next page/i }));
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-systemd-linux",
                "list_units",
                { offset: 50, limit: 50 },
                expect.anything(),
            );
        });

        // Bump page size — should snap back to offset 0.
        await user.selectOptions(
            screen.getByRole("combobox", { name: /page size/i }),
            "100",
        );
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-systemd-linux",
                "list_units",
                { offset: 0, limit: 100 },
                expect.anything(),
            );
        });
    });

    it("offset pagination: form-driven request changes reset offset to 0", async () => {
        mocked.invokePluginRPC.mockResolvedValue({
            ...stubUnits,
            totalCount: 312,
            hasMore: true,
        });

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    requestForm={[
                        {
                            field: "state",
                            kind: "select",
                            label: "State",
                            options: [
                                { value: "", label: "All" },
                                { value: "failed", label: "Failed" },
                            ],
                            default: "",
                        },
                    ]}
                    buildRequest={(form) =>
                        form.state ? { state: form.state } : {}
                    }
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[{ field: "name", label: "Service", primary: true }]}
                    pagination={{ kind: "offset", pageSize: 50 }}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));
        await user.click(screen.getByRole("button", { name: /next page/i }));
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenLastCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-systemd-linux",
                "list_units",
                { offset: 50, limit: 50 },
                expect.anything(),
            );
        });

        // Pick a filter — paging state should snap to offset 0 since
        // the dataset effectively changed.
        await user.selectOptions(
            screen.getByRole("combobox", { name: /state/i }),
            "failed",
        );
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenLastCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-systemd-linux",
                "list_units",
                { state: "failed", offset: 0, limit: 50 },
                expect.anything(),
            );
        });
    });

    // -----------------------------------------------------------------------
    // Pagination — cursor (sys-journald-linux + sys-log-{darwin,windows})
    // -----------------------------------------------------------------------

    it("cursor pagination: Older button fires beforeCursor, Newer fires afterCursor", async () => {
        mocked.invokePluginRPC.mockResolvedValue({
            entries: [
                { timestampUs: 5, message: "five" },
                { timestampUs: 4, message: "four" },
            ],
            prevCursor: "cur-old",
            nextCursor: "cur-new",
        });

        const user = userEvent.setup();
        render(
            <Wrapper>
                <RPCTable<{ entries: Array<{ message: string }> }, { message: string }>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-journald-linux"
                    method="query"
                    rowsFrom={(r) => r.entries}
                    rowKey={(row, idx) => `${row.message}-${idx}`}
                    columns={[
                        { field: "message", label: "Message", primary: true },
                    ]}
                    pagination={{ kind: "cursor" }}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("five"));

        await user.click(screen.getByRole("button", { name: /older/i }));
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-journald-linux",
                "query",
                { beforeCursor: "cur-old" },
                expect.anything(),
            );
        });

        await user.click(screen.getByRole("button", { name: /newer/i }));
        await waitFor(() => {
            expect(mocked.invokePluginRPC).toHaveBeenCalledWith(
                "proj1",
                "agent-a1",
                "com.platypus.sys-journald-linux",
                "query",
                { afterCursor: "cur-new" },
                expect.anything(),
            );
        });
    });

    it("cursor pagination: Older is disabled when no prevCursor in response", async () => {
        mocked.invokePluginRPC.mockResolvedValue({
            entries: [{ timestampUs: 1, message: "only" }],
            // No prevCursor / nextCursor — single-page result.
        });

        render(
            <Wrapper>
                <RPCTable<{ entries: Array<{ message: string }> }, { message: string }>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-journald-linux"
                    method="query"
                    rowsFrom={(r) => r.entries}
                    rowKey={(row, idx) => `${row.message}-${idx}`}
                    columns={[
                        { field: "message", label: "Message", primary: true },
                    ]}
                    pagination={{ kind: "cursor" }}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("only"));
        expect(screen.getByRole("button", { name: /older/i })).toBeDisabled();
    });

    it("primary column gets a bolder font weight than the rest", async () => {
        mocked.invokePluginRPC.mockResolvedValueOnce(stubUnits);

        render(
            <Wrapper>
                <RPCTable<ListUnitsResponse, ListUnitsResponse["units"][number]>
                    projectID="proj1"
                    agentID="agent-a1"
                    pluginID="com.platypus.sys-systemd-linux"
                    method="list_units"
                    rowsFrom={(r) => r.units}
                    rowKey={(row) => row.name}
                    columns={[
                        { field: "name", label: "Service", primary: true },
                        { field: "description", label: "Description" },
                    ]}
                />
            </Wrapper>,
        );

        await waitFor(() => screen.getByText("ssh.service"));
        const primaryCell = screen.getByText("ssh.service").closest("td");
        expect(primaryCell).not.toBeNull();
        expect(primaryCell?.getAttribute("data-primary")).toBe("true");
    });
});

// (Side-effect: vi.fn objects — also unused-export silenced.)
const _ = fireEvent;
void _;
