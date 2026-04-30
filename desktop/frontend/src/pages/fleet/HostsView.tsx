import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LayoutGrid, Rows3, Search } from "lucide-react";
import { useNavigate, useParams, Outlet } from "react-router-dom";

import StatusDot from "../../components/StatusDot";
import { useCurrentProject } from "../../layout/ProjectShell";
import { palette, space } from "../../layout/theme";
import { Host, listHosts } from "../../lib/api";
import { qk } from "../../lib/queryKeys";
import { fromNow, isOnline } from "../../lib/time";
import { usePreference } from "../../lib/preferences";

import { Input } from "@/components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import HostsCardPanel from "./HostsCardPanel";
import HostsPanel from "./HostsPanel";

type HostsLensView = "cards" | "table";
const VIEWS: readonly HostsLensView[] = ["cards", "table"] as const;

const RAIL_PX = 260;

// HostsView is the Hosts sub-tab body. Two states:
//   · No host selected (URL `…/fleet/hosts`): full-width list with
//     a Cards / Table toggle, mounting HostsCardPanel or HostsPanel.
//   · Host selected (URL `…/fleet/hosts/<id>/<activity>`): a
//     compact 260 px rail on the left listing every host (status
//     dot + alias + last-seen), and the rest of the surface filled
//     by the right pane (HostView, rendered via <Outlet />).
//
// The list / rail share one query key so opening a host doesn't
// re-fetch. The rail's selected row scrolls into view on selection
// change so deep-linking to a host past the fold lands the rail
// scrolled to it.
export default function HostsView() {
    const params = useParams<{ hostId?: string }>();
    const hostId = params.hostId;
    if (hostId) return <HostsMasterDetail hostId={hostId} />;
    return <HostsListOnly />;
}

// HostsListOnly is the no-selection state: full-width Cards/Table
// toggle. The toggle preference is stored under the legacy
// `ui.fleet.defaultView` key so users keep their previous default
// across the C4 split (timeline/graph values fall through to "table"
// because those views moved to dedicated sub-tabs).
function HostsListOnly() {
    const [stored, setStored] = usePreference("ui.fleet.defaultView");
    const view: HostsLensView = (VIEWS as readonly string[]).includes(stored)
        ? (stored as HostsLensView)
        : "table";

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <div
                style={{
                    flexShrink: 0,
                    display: "flex",
                    justifyContent: "flex-end",
                    padding: `${space[2]}px ${space[4]}px`,
                    borderBottom: `1px solid ${palette.border}`,
                    background: palette.rail,
                }}
            >
                <span
                    data-testid="fleet-view-toggle"
                    title={`Default view: ${view}. Change the default in Preferences (browser-local).`}
                >
                    <ToggleGroup
                        type="single"
                        variant="outline"
                        size="sm"
                        value={view}
                        onValueChange={(v) => {
                            if (v === "cards" || v === "table") setStored(v);
                        }}
                    >
                        <ToggleGroupItem value="cards" aria-label="Card view">
                            <LayoutGrid className="size-3.5" />
                            Cards
                        </ToggleGroupItem>
                        <ToggleGroupItem value="table" aria-label="Table view">
                            <Rows3 className="size-3.5" />
                            Table
                        </ToggleGroupItem>
                    </ToggleGroup>
                </span>
            </div>
            <div style={{ flex: 1, minHeight: 0, position: "relative" }}>
                <div
                    data-testid="fleet-panel-cards"
                    aria-hidden={view !== "cards"}
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: view === "cards" ? "block" : "none",
                    }}
                >
                    <HostsCardPanel />
                </div>
                <div
                    data-testid="fleet-panel-table"
                    aria-hidden={view !== "table"}
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: view === "table" ? "block" : "none",
                    }}
                >
                    <HostsPanel />
                </div>
            </div>
        </div>
    );
}

function HostsMasterDetail({ hostId }: { hostId: string }) {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const [query, setQuery] = useState("");
    const { data: hosts = [] } = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
    });

    const filtered = useMemo(() => {
        const q = query.trim().toLowerCase();
        if (!q) return hosts;
        return hosts.filter((h) =>
            [h.hostname, h.primary_alias, h.os, h.machine_id, h.primary_ip]
                .filter(Boolean)
                .some((v) => String(v).toLowerCase().includes(q)),
        );
    }, [hosts, query]);

    return (
        <div style={{ display: "flex", height: "100%", minHeight: 0 }}>
            <aside
                data-testid="hosts-rail"
                style={{
                    flexShrink: 0,
                    width: RAIL_PX,
                    borderRight: `1px solid ${palette.border}`,
                    background: palette.rail,
                    display: "flex",
                    flexDirection: "column",
                    minHeight: 0,
                }}
            >
                <div
                    style={{
                        flexShrink: 0,
                        position: "relative",
                        padding: space[2],
                        borderBottom: `1px solid ${palette.border}`,
                    }}
                >
                    <Search
                        aria-hidden
                        className="size-3.5"
                        style={{
                            position: "absolute",
                            left: space[2] + 8,
                            top: "50%",
                            transform: "translateY(-50%)",
                            color: palette.textMuted,
                            pointerEvents: "none",
                        }}
                    />
                    <Input
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder="Search hosts"
                        className="h-7 pl-7 text-xs"
                    />
                </div>
                <div style={{ flex: 1, overflow: "auto", padding: 4 }}>
                    {filtered.map((h) => (
                        <RailRow
                            key={h.id}
                            host={h}
                            active={h.id === hostId}
                            onClick={() =>
                                navigate(
                                    `/projects/${project.slug}/fleet/hosts/${h.id}/files`,
                                )
                            }
                        />
                    ))}
                </div>
            </aside>
            <div style={{ flex: 1, minWidth: 0, minHeight: 0, display: "flex", flexDirection: "column" }}>
                <Outlet />
            </div>
        </div>
    );
}

function RailRow({
    host,
    active,
    onClick,
}: {
    host: Host;
    active: boolean;
    onClick: () => void;
}) {
    const ref = useRef<HTMLButtonElement>(null);
    useEffect(() => {
        if (active) {
            ref.current?.scrollIntoView({ block: "nearest" });
        }
    }, [active]);

    const primary =
        host.primary_alias ||
        host.hostname ||
        host.machine_id?.slice(0, 8) ||
        "unknown";
    const status = isOnline(host.last_seen_at) ? "online" : "offline";

    return (
        <button
            ref={ref}
            type="button"
            onClick={onClick}
            data-testid={`hosts-rail-row-${host.id}`}
            data-active={active || undefined}
            aria-current={active ? "true" : undefined}
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                width: "100%",
                padding: `6px ${space[2]}px`,
                background: active ? palette.surfaceHover : "transparent",
                border: "none",
                borderRadius: 6,
                cursor: "pointer",
                color: palette.textPrimary,
                textAlign: "left",
            }}
        >
            <StatusDot status={status} />
            <span
                style={{
                    flex: 1,
                    minWidth: 0,
                    display: "flex",
                    flexDirection: "column",
                    lineHeight: 1.25,
                }}
            >
                <span
                    style={{
                        fontSize: 12,
                        fontWeight: active ? 600 : 500,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                >
                    {primary}
                </span>
                <span
                    style={{
                        fontSize: 10,
                        color: palette.textMuted,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                >
                    {fromNow(host.last_seen_at)}
                </span>
            </span>
        </button>
    );
}
