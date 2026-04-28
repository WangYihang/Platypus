import { ReactNode, createContext, useContext, useEffect, useMemo, useState } from "react";
import { ArrowDownToLine, ArrowUpFromLine } from "lucide-react";

import { palette, radius, space } from "../layout/theme";
import { getSession, onSessionChange } from "../lib/auth";
import {
    cancelTransfer,
    createTransfersStore,
    type TransferItem,
    type TransferStatus,
} from "../lib/transfers";
import { Button } from "./ui/button";
import StatusPill from "./StatusPill";

// TransfersDrawerContext is a tiny app-level toggle for the right-side
// transfers drawer. Lives at the same layer as GlobalTerminalProvider
// so any leaf (the Files toolbar, the status-bar pill, the terminal
// drawer) can flip it open without prop drilling.
interface TransfersDrawerContextValue {
    open: boolean;
    setOpen: (v: boolean) => void;
    rows: TransferItem[];
    activeCount: number;
}

const TransfersDrawerContext = createContext<TransfersDrawerContextValue | null>(null);

export function TransfersDrawerProvider({ children }: { children: ReactNode }) {
    const [open, setOpen] = useState(false);
    const [rows, setRows] = useState<TransferItem[]>([]);
    const [sessionToken, setSessionToken] = useState<string | null>(
        () => getSession()?.sessionToken ?? null,
    );

    useEffect(() => {
        return onSessionChange(() => {
            setSessionToken(getSession()?.sessionToken ?? null);
        });
    }, []);

    // Re-create the store on login/logout so we don't keep a stale
    // auth header bound. Scoped global (no project / host filter) so
    // the drawer is the cross-workspace transfer log.
    const store = useMemo(
        () => (sessionToken ? createTransfersStore({}) : null),
        [sessionToken],
    );

    useEffect(() => {
        if (!store) {
            setRows([]);
            return;
        }
        let cancelled = false;
        store.load().catch(() => {
            // Surface failure quietly — the drawer renders an empty
            // state and the per-host transfers tab repeats the load.
        });
        const unsub = store.subscribe((next) => {
            if (cancelled) return;
            setRows(next);
        });
        return () => {
            cancelled = true;
            unsub();
            store.dispose();
        };
    }, [store]);

    const activeCount = rows.filter((r) => r.status === "running" || r.status === "pending").length;

    const value = useMemo<TransfersDrawerContextValue>(
        () => ({ open, setOpen, rows, activeCount }),
        [open, rows, activeCount],
    );
    return (
        <TransfersDrawerContext.Provider value={value}>
            {children}
        </TransfersDrawerContext.Provider>
    );
}

export function useTransfersDrawer(): TransfersDrawerContextValue {
    const ctx = useContext(TransfersDrawerContext);
    if (!ctx) {
        // Outside the provider (login screens, the projects landing
        // route) the drawer is meaningless. Returning a no-op shape
        // lets the pill render conditionally without throwing.
        return { open: false, setOpen: () => {}, rows: [], activeCount: 0 };
    }
    return ctx;
}

const STATUS_TONES: Record<
    TransferStatus,
    "neutral" | "success" | "warning" | "danger" | "info"
> = {
    pending: "neutral",
    running: "info",
    done: "success",
    failed: "danger",
    canceled: "warning",
};

const TERMINAL: ReadonlySet<TransferStatus> = new Set([
    "done",
    "failed",
    "canceled",
]);

function formatBytes(n: number): string {
    if (!Number.isFinite(n) || n <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let idx = 0;
    let v = n;
    while (v >= 1024 && idx < units.length - 1) {
        v /= 1024;
        idx++;
    }
    return `${v.toFixed(v >= 100 || idx === 0 ? 0 : 1)} ${units[idx]}`;
}

function formatPaths(paths: string[]): string {
    if (paths.length === 0) return "";
    if (paths.length === 1) return paths[0];
    return `${paths[0]} (+${paths.length - 1} more)`;
}

function progressPct(it: TransferItem): number {
    if (it.total_bytes <= 0) return it.status === "done" ? 100 : 0;
    return Math.min(100, Math.round((it.bytes_transferred / it.total_bytes) * 100));
}

// TransfersPill is the status-bar trigger for the right drawer.
// Mirrors TerminalsPill: a tiny chip that shows the count of active
// transfers; clicking it toggles the drawer. We keep it visible even
// at zero so operators have a discoverable place to find historical
// transfers (matches the "always-on right drawer" UX the user asked
// for, without forcing a giant panel onto the layout when the log
// is empty).
export default function TransfersPill() {
    const { rows, activeCount, open, setOpen } = useTransfersDrawer();
    const total = rows.length;
    return (
        <button
            type="button"
            data-testid="transfers-pill"
            aria-label={
                activeCount > 0
                    ? `${activeCount} transfer${activeCount === 1 ? "" : "s"} in progress`
                    : "Open transfers drawer"
            }
            aria-pressed={open}
            title={
                activeCount > 0
                    ? `${activeCount} active · ${total} total — click to open the transfers drawer`
                    : "Open transfers drawer"
            }
            onClick={() => setOpen(!open)}
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
                padding: "1px 8px",
                background: open ? palette.surfaceHover : palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.pill,
                color: activeCount > 0 ? palette.info : palette.textSecondary,
                fontSize: 11,
                cursor: "pointer",
            }}
        >
            <ArrowDownToLine className="size-3" />
            <span>{activeCount > 0 ? `${activeCount}` : total > 0 ? `${total}` : "0"}</span>
        </button>
    );
}

// TransfersDrawer is a right-anchored side panel that lists every
// transfer in the active workspace. It's a persistent surface in the
// shell — the operator opens it once, leaves it open, and watches
// progress while they continue working. Designed to slide over the
// outlet rather than push it: pushing would reflow every page on
// drawer open.
export function TransfersDrawer() {
    const { open, setOpen, rows } = useTransfersDrawer();
    if (!open) return null;
    return (
        <div
            data-testid="transfers-drawer"
            role="dialog"
            aria-label="File transfers"
            style={{
                position: "absolute",
                top: 0,
                right: 0,
                bottom: 0,
                width: 420,
                maxWidth: "90vw",
                background: palette.surface,
                borderLeft: `1px solid ${palette.border}`,
                display: "flex",
                flexDirection: "column",
                zIndex: 30,
                boxShadow: "-12px 0 32px rgba(0,0,0,0.35)",
            }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: `${space[3]}px ${space[4]}px`,
                    borderBottom: `1px solid ${palette.border}`,
                }}
            >
                <span
                    style={{
                        fontSize: 13,
                        fontWeight: 600,
                        color: palette.textPrimary,
                    }}
                >
                    Transfers
                </span>
                <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setOpen(false)}
                    aria-label="Close transfers drawer"
                >
                    Close
                </Button>
            </div>
            <div style={{ flex: 1, overflowY: "auto", padding: space[2] }}>
                {rows.length === 0 ? (
                    <div
                        style={{
                            padding: space[6],
                            color: palette.textMuted,
                            fontSize: 12,
                            textAlign: "center",
                        }}
                    >
                        No transfers yet. Downloads and uploads will appear here.
                    </div>
                ) : (
                    rows.map((it) => <TransferRow key={it.id} item={it} />)
                )}
            </div>
        </div>
    );
}

function TransferRow({ item }: { item: TransferItem }) {
    const Icon = item.direction === "download" ? ArrowDownToLine : ArrowUpFromLine;
    const pct = progressPct(item);
    const fill =
        item.status === "failed"
            ? palette.danger
            : item.status === "canceled"
                ? palette.warning
                : item.status === "done"
                    ? palette.success
                    : palette.info;
    return (
        <div
            data-testid="transfers-drawer-row"
            style={{
                padding: `${space[3]}px ${space[3]}px`,
                borderBottom: `1px solid ${palette.border}`,
                display: "flex",
                flexDirection: "column",
                gap: space[2],
            }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    fontSize: 12,
                    color: palette.textPrimary,
                }}
            >
                <Icon
                    className="size-3.5"
                    style={{ color: palette.textMuted, flexShrink: 0 }}
                />
                <span
                    title={item.paths.join("\n")}
                    style={{
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        flex: 1,
                        minWidth: 0,
                    }}
                >
                    {formatPaths(item.paths)}
                </span>
                <StatusPill tone={STATUS_TONES[item.status]}>{item.status}</StatusPill>
            </div>
            <div
                style={{
                    position: "relative",
                    width: "100%",
                    height: 4,
                    background: palette.border,
                    borderRadius: radius.pill,
                    overflow: "hidden",
                }}
            >
                <div
                    style={{
                        width: `${pct}%`,
                        height: "100%",
                        background: fill,
                        transition: "width 200ms ease-out",
                    }}
                />
            </div>
            <div
                style={{
                    display: "flex",
                    justifyContent: "space-between",
                    fontSize: 11,
                    color: palette.textMuted,
                }}
            >
                <span>
                    {formatBytes(item.bytes_transferred)}
                    {item.total_bytes > 0
                        ? ` / ${formatBytes(item.total_bytes)}`
                        : ""}
                    {" · "}
                    {pct}%
                </span>
                {!TERMINAL.has(item.status) ? (
                    <button
                        type="button"
                        onClick={() =>
                            cancelTransfer({
                                projectId: item.project_id,
                                transferId: item.id,
                            }).catch(() => {})
                        }
                        style={{
                            background: "transparent",
                            border: "none",
                            color: palette.textMuted,
                            fontSize: 11,
                            cursor: "pointer",
                            padding: 0,
                        }}
                    >
                        Cancel
                    </button>
                ) : (
                    <span>{new Date(item.started_at).toLocaleTimeString()}</span>
                )}
            </div>
            {item.error_message ? (
                <span
                    style={{
                        fontSize: 11,
                        color: palette.danger,
                        wordBreak: "break-word",
                    }}
                >
                    {item.error_message}
                </span>
            ) : null}
        </div>
    );
}
