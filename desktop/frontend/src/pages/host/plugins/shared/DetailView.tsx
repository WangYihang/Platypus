// <DetailView> — sibling of <RPCTable> for show_unit-style RPCs.
// Calls one plugin RPC, renders the response as a 2-column key/value
// table. No filters, no actions — pure read-only inspection. Used
// by sys-systemd-linux's "View details" row-action.

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";

import EmptyState from "../../../../components/EmptyState";
import { palette, space } from "../../../../layout/theme";
import { humanizeError } from "../../../../lib/humanizeError";
import { invokePluginRPC } from "../../../../lib/api/agents/plugins";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

export interface DetailViewProps<TResponse> {
    projectID: string;
    agentID: string;
    pluginID: string;
    method: string;
    request: unknown;
    /** Extract the {key: value} record from the RPC response. */
    rowsFrom: (resp: TResponse) => Record<string, string | number | boolean>;
    /** Optional: highlight one key (e.g. "Id" or "Name"). */
    primaryKey?: string;
    active?: boolean;
}

export default function DetailView<TResponse>(props: DetailViewProps<TResponse>) {
    const {
        projectID,
        agentID,
        pluginID,
        method,
        request,
        rowsFrom,
        primaryKey,
        active = true,
    } = props;

    const query = useQuery({
        queryKey: ["plugin-detail", projectID, agentID, pluginID, method, request] as const,
        queryFn: ({ signal }) =>
            invokePluginRPC<TResponse>(
                projectID,
                agentID,
                pluginID,
                method,
                request,
                { signal },
            ),
        enabled: active && agentID !== "",
        refetchOnWindowFocus: false,
        retry: false,
    });

    const entries = useMemo<Array<[string, string | number | boolean]>>(() => {
        if (!query.data) return [];
        const obj = rowsFrom(query.data);
        const out = Object.entries(obj);
        // Sort by key so the rendering is deterministic.
        out.sort((a, b) => {
            // Primary key floats to the top.
            if (a[0] === primaryKey) return -1;
            if (b[0] === primaryKey) return 1;
            return a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0;
        });
        return out;
    }, [query.data, rowsFrom, primaryKey]);

    if (query.isLoading) {
        return (
            <div
                style={{
                    display: "flex",
                    justifyContent: "center",
                    padding: space[6],
                }}
            >
                <Loader2 className="size-5 animate-spin" />
            </div>
        );
    }
    if (query.error) {
        return (
            <EmptyState
                title="Couldn't load"
                description={humanizeError(query.error)}
            />
        );
    }
    if (entries.length === 0) {
        return (
            <EmptyState
                title="Nothing to show"
                description="The plugin returned no properties."
            />
        );
    }
    return (
        <Table>
            <TableHeader>
                <TableRow>
                    <TableHead style={{ width: 240 }}>Key</TableHead>
                    <TableHead>Value</TableHead>
                </TableRow>
            </TableHeader>
            <TableBody>
                {entries.map(([k, v]) => (
                    <TableRow key={k} data-primary={k === primaryKey ? "true" : "false"}>
                        <TableCell
                            style={{
                                fontWeight: k === primaryKey ? 600 : 500,
                                color: palette.textPrimary,
                                fontFamily: "monospace",
                                fontSize: 12,
                            }}
                        >
                            {k}
                        </TableCell>
                        <TableCell
                            style={{
                                color: palette.textPrimary,
                                fontFamily: "monospace",
                                fontSize: 12,
                                whiteSpace: "pre-wrap",
                                wordBreak: "break-all",
                            }}
                        >
                            {String(v)}
                        </TableCell>
                    </TableRow>
                ))}
            </TableBody>
        </Table>
    );
}
