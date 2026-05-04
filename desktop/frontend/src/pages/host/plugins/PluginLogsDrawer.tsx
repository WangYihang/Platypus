import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";

import {
    Sheet,
    SheetContent,
    SheetDescription,
    SheetHeader,
    SheetTitle,
} from "@/components/ui/sheet";
import { ScrollArea } from "@/components/ui/scroll-area";

import { palette, space } from "../../../layout/theme";
import { humanizeError } from "../../../lib/humanizeError";
import {
    InstalledPlugin,
    PluginLogEntry,
    pluginLogs,
} from "../../../lib/api/agents/plugins";

interface Props {
    projectID: string;
    /** The host's agent_id (cert SAN). The logs endpoint keys on this. */
    agentID: string;
    /** Open when non-null. Closing fires onClose; the parent unsets
     *  this back to null. */
    plugin: InstalledPlugin | null;
    onClose: () => void;
}

// PluginLogsDrawer tails the agent's per-plugin in-memory log ring.
// Opens as a Radix Sheet on the right edge so it doesn't displace
// the rest of the host page. Refreshes on open + every 5s while
// open.
//
// `tail=200` matches the agent's default ring cap; bumping it
// requires the agent to widen its buffer first so we don't ask for
// more than is kept.
const TAIL_LINES = 200;
const REFRESH_INTERVAL_MS = 5_000;

export default function PluginLogsDrawer({
    projectID,
    agentID,
    plugin,
    onClose,
}: Props) {
    const open = plugin !== null;

    const logs = useQuery({
        queryKey: [
            "agent-plugin-logs",
            projectID,
            agentID,
            plugin?.id ?? "",
        ],
        queryFn: () => pluginLogs(projectID, agentID, plugin!.id, TAIL_LINES),
        enabled: open && agentID !== "",
        refetchInterval: open ? REFRESH_INTERVAL_MS : false,
        refetchOnWindowFocus: false,
        retry: false,
    });

    return (
        <Sheet
            open={open}
            onOpenChange={(next) => {
                if (!next) onClose();
            }}
        >
            <SheetContent side="right" className="w-[640px] sm:max-w-none">
                <SheetHeader>
                    <SheetTitle>{plugin?.name ?? "Plugin logs"}</SheetTitle>
                    <SheetDescription>
                        Most recent {TAIL_LINES} entries from this plugin's
                        log ring on the agent. Refreshes every {REFRESH_INTERVAL_MS / 1000}s.
                    </SheetDescription>
                </SheetHeader>

                {logs.isLoading ? (
                    <div
                        style={{
                            display: "flex",
                            justifyContent: "center",
                            padding: space[6],
                        }}
                    >
                        <Loader2 className="size-5 animate-spin" />
                    </div>
                ) : logs.error ? (
                    <div style={{ padding: space[4], color: palette.danger }}>
                        {humanizeError(logs.error)}
                    </div>
                ) : (
                    <ScrollArea className="h-[calc(100vh-180px)] px-4">
                        {(logs.data ?? []).length === 0 ? (
                            <div
                                style={{
                                    fontSize: 12,
                                    color: palette.textMuted,
                                    padding: space[4],
                                }}
                            >
                                No log entries yet.
                            </div>
                        ) : (
                            <ol
                                style={{
                                    listStyle: "none",
                                    padding: 0,
                                    margin: 0,
                                    display: "flex",
                                    flexDirection: "column",
                                    gap: 2,
                                }}
                            >
                                {(logs.data ?? []).map((e, i) => (
                                    <LogLine key={i} entry={e} />
                                ))}
                            </ol>
                        )}
                    </ScrollArea>
                )}
            </SheetContent>
        </Sheet>
    );
}

function LogLine({ entry }: { entry: PluginLogEntry }) {
    const ts = new Date(Math.floor(entry.unix_nano / 1e6));
    return (
        <li
            style={{
                fontFamily: "monospace",
                fontSize: 11,
                color: levelColor(entry.level),
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
            }}
        >
            <span style={{ color: palette.textMuted }}>
                {ts.toISOString().replace("T", " ").replace(/\..+/, "")}
            </span>{" "}
            <span style={{ fontWeight: 600 }}>{entry.level.toUpperCase()}</span>{" "}
            {entry.message}
            {entry.correlation_id && (
                <span style={{ color: palette.textMuted }}> ({entry.correlation_id})</span>
            )}
        </li>
    );
}

function levelColor(level: string): string {
    switch (level.toLowerCase()) {
        case "error":
            return palette.danger;
        case "warn":
        case "warning":
            return palette.warning ?? palette.textPrimary;
        default:
            return palette.textPrimary;
    }
}
