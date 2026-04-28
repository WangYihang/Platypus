import { ReactNode, useEffect, useRef, useState } from "react";

import { EventsOff, EventsOn } from "@wails/runtime/runtime";
import { palette, radius, space } from "../layout/theme";
import { getSession, onActiveChange, onSessionChange } from "../lib/auth";
import { getActiveServer, onServersChange } from "../lib/servers";
import { ServerInfo, getServerInfo } from "../lib/api";
import { formatBytes, formatUptimeSeconds } from "../lib/format";
import TerminalsPill from "../terminal/TerminalsPill";
import Mono from "./Mono";
import StatusDot from "./StatusDot";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

// Refresh cadence: 1 Hz so memory / goroutines / uptime tick like a
// proper telemetry strip. The endpoint is small (one cheap COUNT
// roll-up + one ReadMemStats) so the bandwidth and CPU cost are
// negligible. If we ever scale past N≈hundreds of concurrent
// dashboards, bucket the chatty fields onto a separate cheaper
// endpoint.
const POLL_MS = 1_000;

// StatusBar is pinned to the bottom of ShellChrome. Three zones:
//   · left   — local build (app version + commit)
//   · center — connection health (dot + server host)
//   · right  — remote counts + server version
// On fetch failure the last-known counts stay on screen; the dot flips
// to `error` so the UI stays legible rather than flashing empty.
export default function StatusBar() {
    const [session, setSession] = useState(() => getSession());
    const [activeName, setActiveName] = useState(() => getActiveServer()?.name ?? null);
    const [info, setInfo] = useState<ServerInfo | null>(null);
    const [online, setOnline] = useState<"online" | "offline" | "error">("offline");
    const [lastPollAt, setLastPollAt] = useState<number | null>(null);
    const [lastPollMs, setLastPollMs] = useState<number | null>(null);
    const [lastError, setLastError] = useState<string | null>(null);
    const timerRef = useRef<number | null>(null);

    // Keep local session / active-profile state in sync with login,
    // logout, server switch, rename — so the bar always names the
    // workspace the user is currently looking at.
    useEffect(() => {
        const unsubs = [
            onSessionChange(() => setSession(getSession())),
            onActiveChange(() => {
                setSession(getSession());
                setActiveName(getActiveServer()?.name ?? null);
            }),
            onServersChange(() => setActiveName(getActiveServer()?.name ?? null)),
        ];
        return () => unsubs.forEach((u) => u());
    }, []);

    useEffect(() => {
        if (!session) {
            setInfo(null);
            setOnline("offline");
            return;
        }

        let cancelled = false;
        const tick = async () => {
            const start = Date.now();
            try {
                const fresh = await getServerInfo();
                if (cancelled) return;
                setInfo(fresh);
                setOnline("online");
                setLastError(null);
                setLastPollAt(Date.now());
                setLastPollMs(Date.now() - start);
            } catch (err) {
                if (cancelled) return;
                setOnline("error");
                setLastError(err instanceof Error ? err.message : String(err));
                setLastPollAt(Date.now());
                setLastPollMs(Date.now() - start);
            }
        };

        void tick();
        timerRef.current = window.setInterval(tick, POLL_MS);

        // Refresh immediately when the server reports client churn —
        // the Wails app emits these, and runtime.web.ts emits them too
        // once the notify bridge is wired up.
        const onChurn = () => void tick();
        EventsOn("notify:client_connected", onChurn);
        EventsOn("notify:client_duplicated", onChurn);

        return () => {
            cancelled = true;
            if (timerRef.current !== null) {
                window.clearInterval(timerRef.current);
                timerRef.current = null;
            }
            EventsOff("notify:client_connected");
            EventsOff("notify:client_duplicated");
        };
    }, [session]);

    const serverHost = (() => {
        if (!session) return "not connected";
        try {
            return new URL(session.serverURL).host;
        } catch {
            return session.serverURL;
        }
    })();

    return (
        <div
            role="status"
            data-testid="status-bar"
            style={{
                flexShrink: 0,
                height: 28,
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: space[4],
                padding: `0 ${space[3]}px`,
                background: palette.rail,
                borderTop: `1px solid ${palette.border}`,
                color: palette.textMuted,
                fontSize: 11,
                lineHeight: 1.6,
            }}
        >
            <div style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                <span
                    style={{
                        display: "inline-block",
                        width: 6,
                        height: 6,
                        borderRadius: radius.pill,
                        background: palette.accent,
                        flexShrink: 0,
                    }}
                />
                <span style={{ color: palette.textSecondary, fontWeight: 500 }}>Platypus</span>
                <Mono size={11} color={palette.textMuted}>
                    v{__APP_VERSION__}
                </Mono>
                <span style={{ color: palette.border }}>·</span>
                <Mono size={11} color={palette.textMuted}>
                    {__APP_COMMIT__.slice(0, 7)}
                </Mono>
            </div>

            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    minWidth: 0,
                    flex: "0 1 auto",
                }}
            >
                <Popover>
                    <PopoverTrigger asChild>
                        <button
                            type="button"
                            data-testid="status-bar-status-trigger"
                            aria-label="Server status detail"
                            style={{
                                background: "none",
                                border: "none",
                                padding: 0,
                                margin: 0,
                                cursor: "pointer",
                                display: "inline-flex",
                                alignItems: "center",
                            }}
                        >
                            <StatusDot
                                status={online === "error" ? "error" : online}
                                title={
                                    online === "online"
                                        ? "server reachable"
                                        : online === "error"
                                          ? "server unreachable"
                                          : "not connected"
                                }
                            />
                        </button>
                    </PopoverTrigger>
                    <PopoverContent side="top" align="start" className="w-[280px] text-xs">
                        <div className="space-y-1">
                            <div>
                                <span className="text-text-muted">Status: </span>
                                <span className="text-text-primary">
                                    {online === "online"
                                        ? "Reachable"
                                        : online === "error"
                                          ? "Unreachable"
                                          : "Not connected"}
                                </span>
                            </div>
                            {lastPollAt && (
                                <div>
                                    <span className="text-text-muted">Last poll: </span>
                                    <span className="text-text-primary">
                                        {new Date(lastPollAt).toLocaleTimeString()}
                                    </span>
                                    {lastPollMs !== null && (
                                        <span className="text-text-muted">
                                            {" "}({lastPollMs} ms)
                                        </span>
                                    )}
                                </div>
                            )}
                            {lastError && (
                                <div className="break-words text-danger">
                                    <span className="text-text-muted">Last error: </span>
                                    {lastError}
                                </div>
                            )}
                            <div>
                                <span className="text-text-muted">Server: </span>
                                <span className="text-text-primary">{serverHost}</span>
                            </div>
                            {info?.public_addr && (
                                <div data-testid="status-bar-ingress-popover">
                                    <span className="text-text-muted">Ingress: </span>
                                    <span className="text-text-primary">
                                        {info.public_addr}
                                    </span>
                                </div>
                            )}
                        </div>
                    </PopoverContent>
                </Popover>
                {activeName && (
                    <>
                        <span
                            style={{
                                color: palette.textSecondary,
                                fontWeight: 500,
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                                minWidth: 0,
                            }}
                            title={activeName}
                        >
                            {activeName}
                        </span>
                        <span style={{ color: palette.border, flexShrink: 0 }}>·</span>
                    </>
                )}
                <span
                    style={{
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        minWidth: 0,
                    }}
                    title={serverHost}
                >
                    <Mono size={11} color={palette.textMuted}>
                        {serverHost}
                    </Mono>
                </span>
                {session?.user && (
                    <>
                        <span style={{ color: palette.border, flexShrink: 0 }}>·</span>
                        <span
                            data-testid="status-bar-user"
                            style={{
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                                minWidth: 0,
                            }}
                            title={session.user.username}
                        >
                            <Mono size={11} color={palette.textMuted}>
                                {session.user.username}
                            </Mono>
                        </span>
                    </>
                )}
            </div>

            <div style={{ display: "flex", alignItems: "center", gap: space[3] }}>
                <TerminalsPill />
                <RuntimePills info={info} />
                <CountPills info={info} />
                <VersionLinks info={info} />
            </div>
        </div>
    );
}

// --- right-zone sub-components ----------------------------------------
// Each renders nothing (or "—") until the first /info response lands.
// They tick at 1 Hz alongside the parent's poll loop because the
// parent re-renders on every tick.

function RuntimePills({ info }: { info: ServerInfo | null }) {
    // Uptime needs Date.now(), which would change every tick — we
    // recompute on each render so the pill counts up live without
    // a separate timer. Anchored to started_at_unix so the
    // arithmetic is integer seconds, no Date.parse() drift.
    const uptimeSecs =
        info?.started_at_unix !== undefined
            ? Math.floor(Date.now() / 1000) - info.started_at_unix
            : null;

    return (
        <>
            <Pill
                testid="status-bar-mem"
                title="Resident memory (runtime.MemStats.Alloc)"
            >
                <span style={{ color: palette.textMuted }}>mem</span>
                <Mono size={11} color={palette.textPrimary}>
                    {formatBytes(info?.mem_alloc_bytes)}
                </Mono>
            </Pill>
            <Sep />
            <Pill
                testid="status-bar-goroutines"
                title="Active goroutines (runtime.NumGoroutine)"
            >
                <span style={{ color: palette.textMuted }}>grtn</span>
                <Mono size={11} color={palette.textPrimary}>
                    {info?.goroutines ?? "—"}
                </Mono>
            </Pill>
            <Sep />
            <Pill
                testid="status-bar-uptime"
                title={
                    info?.started_at
                        ? `Process started at ${info.started_at}`
                        : "Server uptime"
                }
            >
                <span style={{ color: palette.textMuted }}>up</span>
                <Mono size={11} color={palette.textPrimary}>
                    {formatUptimeSeconds(uptimeSecs)}
                </Mono>
            </Pill>
        </>
    );
}

function CountPills({ info }: { info: ServerInfo | null }) {
    return (
        <>
            <Sep />
            <Pill
                testid="status-bar-hosts"
                title="Live / total hosts (live = last_seen within 60s)"
            >
                <span style={{ color: palette.textMuted }}>Hosts</span>
                <Mono size={11} color={palette.textPrimary}>
                    {info?.live_host_count ?? "—"}
                </Mono>
                <span style={{ color: palette.border }}>/</span>
                <Mono size={11} color={palette.textSecondary}>
                    {info?.host_count ?? "—"}
                </Mono>
            </Pill>
            <Sep />
            <Pill
                testid="status-bar-sessions"
                title="Live / total sessions (live = no disconnected_at stamp)"
            >
                <span style={{ color: palette.textMuted }}>Sessions</span>
                <Mono size={11} color={palette.textPrimary}>
                    {info?.live_session_count ?? info?.session_count ?? "—"}
                </Mono>
                <span style={{ color: palette.border }}>/</span>
                <Mono size={11} color={palette.textSecondary}>
                    {info?.total_session_count ?? "—"}
                </Mono>
            </Pill>
        </>
    );
}

function VersionLinks({ info }: { info: ServerInfo | null }) {
    // git_repo defaults to the canonical Platypus repo when the
    // server doesn't report one — older builds didn't include the
    // field. The web version comes from vite's __APP_VERSION__
    // build-time global.
    const repo = info?.git_repo || "WangYihang/Platypus";
    const serverVer = info?.version;
    const webVer = __APP_VERSION__;

    return (
        <>
            <Sep />
            <ReleaseLink
                testid="status-bar-server-version"
                repo={repo}
                version={serverVer}
                label="server"
            />
            <Sep />
            <ReleaseLink
                testid="status-bar-web-version"
                repo={repo}
                version={webVer}
                label="web"
            />
        </>
    );
}

function Pill({
    testid,
    title,
    children,
}: {
    testid: string;
    title?: string;
    children: ReactNode;
}) {
    return (
        <span
            data-testid={testid}
            title={title}
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
                whiteSpace: "nowrap",
            }}
        >
            {children}
        </span>
    );
}

function Sep() {
    return <span style={{ color: palette.border, flexShrink: 0 }}>·</span>;
}

function ReleaseLink({
    testid,
    repo,
    version,
    label,
}: {
    testid: string;
    repo: string;
    version: string | undefined;
    label: string;
}) {
    if (!version) {
        return (
            <Pill testid={testid} title={`${label} version unknown`}>
                <span style={{ color: palette.textMuted }}>{label}</span>
                <Mono size={11} color={palette.textSecondary}>
                    —
                </Mono>
            </Pill>
        );
    }
    const href = `https://github.com/${repo}/releases/tag/v${version}`;
    return (
        <a
            data-testid={testid}
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            title={`Open ${label} v${version} release notes on GitHub`}
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
                whiteSpace: "nowrap",
                color: palette.textMuted,
                textDecoration: "none",
            }}
            onMouseEnter={(e) => (e.currentTarget.style.color = palette.textPrimary)}
            onMouseLeave={(e) => (e.currentTarget.style.color = palette.textMuted)}
        >
            <span>{label}</span>
            <Mono size={11} color="inherit">
                v{version}
            </Mono>
        </a>
    );
}
