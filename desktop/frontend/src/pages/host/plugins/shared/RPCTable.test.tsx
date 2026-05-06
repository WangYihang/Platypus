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
