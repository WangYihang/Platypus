import { useMemo } from "react";
import { TerminalSquare } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { palette, radius, space } from "../layout/theme";
import { colorForId } from "../lib/colors";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

import { ShellEntry, useGlobalTerminalSafe } from "./GlobalTerminalContext";

// TerminalsPill is the cross-host index for open shells. The drawer
// is host-scoped now, so without this pill operators would lose
// sight of shells on hosts they're not currently looking at. The
// pill lives in the status bar and:
//
//   1. Renders nothing when no shells are open, so the bar stays
//      tidy on a fresh project.
//   2. Shows the total open-shell count when at least one exists.
//   3. On click, opens a popover that lists shells grouped by host;
//      each row carries the same colorForId(hostId) the drawer
//      uses, so colours match across surfaces.
//   4. Clicking an entry navigates to that host's info page and
//      activates the shell + opens the drawer, so the operator
//      goes straight from "I have N shells running" to "I'm
//      looking at the right one".
export default function TerminalsPill() {
    // The pill lives in StatusBar, which renders even on routes
    // outside the project shell (e.g. login screens) where the
    // global terminal provider isn't mounted. useGlobalTerminalSafe
    // returns null instead of throwing — when the provider is
    // missing there are by definition no shells, so we just render
    // nothing.
    const ctx = useGlobalTerminalSafe();
    const navigate = useNavigate();

    const shells = ctx?.shells ?? [];
    const groups = useMemo(() => groupByHost(shells), [shells]);

    if (!ctx || shells.length === 0) return null;

    function jumpToShell(s: ShellEntry) {
        ctx!.setActive(s.id);
        ctx!.openDrawer();
        navigate(`/projects/${s.projectSlug}/fleet/hosts/${s.hostId}/files`);
    }

    // Pill renders only when shells.length > 0, so it's always
    // "active". Mirrors TransfersPill's active styling so the two
    // chips read as the same family in the status bar.
    return (
        <Popover>
            <PopoverTrigger asChild>
                <button
                    type="button"
                    data-testid="terminals-pill"
                    data-active="true"
                    aria-label={`${shells.length} open terminal${shells.length === 1 ? "" : "s"}`}
                    title="Open terminals across hosts"
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: 5,
                        padding: "2px 10px",
                        background: palette.infoSoft,
                        border: `1px solid ${palette.info}`,
                        borderRadius: radius.pill,
                        color: palette.info,
                        fontSize: 12,
                        fontWeight: 600,
                        cursor: "pointer",
                    }}
                >
                    <span
                        aria-hidden
                        style={{
                            width: 6,
                            height: 6,
                            borderRadius: 999,
                            background: palette.info,
                        }}
                    />
                    <TerminalSquare className="size-3.5" />
                    <span>{shells.length}</span>
                </button>
            </PopoverTrigger>
            <PopoverContent side="top" align="end" className="w-[260px] p-1">
                <div
                    style={{
                        padding: `${space[1]}px ${space[2]}px ${space[2]}px`,
                        fontSize: 11,
                        color: palette.textMuted,
                        borderBottom: `1px solid ${palette.border}`,
                    }}
                >
                    Open terminals · {shells.length}
                </div>
                <div
                    style={{
                        maxHeight: 320,
                        overflowY: "auto",
                        padding: space[1],
                        display: "flex",
                        flexDirection: "column",
                        gap: space[1],
                    }}
                >
                    {groups.map((g) => (
                        <HostGroup
                            key={g.hostId}
                            group={g}
                            onSelect={jumpToShell}
                        />
                    ))}
                </div>
            </PopoverContent>
        </Popover>
    );
}

interface ShellGroup {
    hostId: string;
    hostLabel: string;
    shells: ShellEntry[];
}

function groupByHost(shells: ShellEntry[]): ShellGroup[] {
    const map = new Map<string, ShellGroup>();
    for (const s of shells) {
        const existing = map.get(s.hostId);
        if (existing) {
            existing.shells.push(s);
            continue;
        }
        // Strip the "· N" multi-shell suffix so the group label
        // shows the host name once, not the first shell's index.
        const baseLabel = s.label.replace(/\s+·\s+\d+$/, "");
        map.set(s.hostId, {
            hostId: s.hostId,
            hostLabel: baseLabel,
            shells: [s],
        });
    }
    return [...map.values()];
}

function HostGroup({
    group,
    onSelect,
}: {
    group: ShellGroup;
    onSelect: (s: ShellEntry) => void;
}) {
    const accent = colorForId(group.hostId);
    return (
        <div>
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    padding: `${space[1]}px ${space[2]}px`,
                    fontSize: 11,
                    fontWeight: 500,
                    color: palette.textSecondary,
                }}
            >
                <span
                    aria-hidden
                    style={{
                        width: 8,
                        height: 8,
                        borderRadius: 999,
                        background: accent,
                        flexShrink: 0,
                    }}
                />
                <span
                    style={{
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                    }}
                    title={group.hostLabel}
                >
                    {group.hostLabel}
                </span>
            </div>
            <div style={{ display: "flex", flexDirection: "column" }}>
                {group.shells.map((s) => (
                    <button
                        key={s.id}
                        type="button"
                        onClick={() => onSelect(s)}
                        style={{
                            display: "flex",
                            alignItems: "center",
                            gap: space[2],
                            padding: `${space[1]}px ${space[2]}px ${space[1]}px ${space[5]}px`,
                            background: "transparent",
                            border: "none",
                            color: palette.textPrimary,
                            fontSize: 12,
                            cursor: "pointer",
                            textAlign: "left",
                            borderRadius: radius.sm,
                        }}
                        onMouseEnter={(e) =>
                            (e.currentTarget.style.background = palette.surfaceHover)
                        }
                        onMouseLeave={(e) =>
                            (e.currentTarget.style.background = "transparent")
                        }
                    >
                        <TerminalSquare
                            className="size-3"
                            style={{ color: palette.textMuted, flexShrink: 0 }}
                        />
                        <span
                            style={{
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                            }}
                            title={s.label}
                        >
                            {s.label}
                        </span>
                    </button>
                ))}
            </div>
        </div>
    );
}
