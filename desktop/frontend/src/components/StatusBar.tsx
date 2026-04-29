import { ReactNode, useEffect, useRef, useState } from "react";

import { EventsOff, EventsOn } from "@wails/runtime/runtime";
import { palette, radius, space } from "../layout/theme";
import { getSession, onActiveChange, onSessionChange } from "../lib/auth";
import { getActiveServer, onServersChange } from "../lib/servers";
import { ServerInfo, getServerInfo } from "../lib/api";
import { formatBytes, formatUptimeSeconds } from "../lib/format";
import TerminalsPill from "../terminal/TerminalsPill";
import TransfersPill from "./TransfersPill";
import TransferThroughputPill from "./TransferThroughputPill";
import Mono from "./Mono";
import Sparkline from "./Sparkline";
import StatusDot from "./StatusDot";
import UtcClock from "./UtcClock";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

// Refresh cadence: 1 Hz so memory / goroutines / uptime tick like a
// proper telemetry strip. The endpoint is small (one cheap COUNT
// roll-up + one ReadMemStats) so the bandwidth and CPU cost are
// negligible. If we ever scale past N≈hundreds of concurrent
// dashboards, bucket the chatty fields onto a separate cheaper
// endpoint.
const POLL_MS = 1_000;

// HISTORY_SIZE bounds the in-memory ring per metric the right-hand
// pills sparkline against. 60 samples × 1 Hz = the last minute, which
// matches the per-host topology sparklines.
const HISTORY_SIZE = 60;

// StatusBar is pinned to the bottom of ShellChrome. Three zones:
//   · left   — brand + connection health (dot + server host)
//   · center — global-action pills (terminals, transfers)
//   · right  — runtime telemetry + counts + version
// On fetch failure the last-known counts stay on screen; the dot flips
// to `error` so the UI stays legible rather than flashing empty.
//
// Redesign notes:
//   * The old build pill on the left ("Platypus v0.0.0 · dev") was
//     redundant with the right-hand "server vX.Y" link, so the brand
//     is now pure text and the version chip is single-source.
//   * The standalone "web vX.Y" pill was always 0.0.0 (vite reads
//     package.json, which we don't bump) so it landed empty. Drop it.
//   * The active-server profile name is hidden when it equals the
//     server URL host (the common case in dev — the user adds a
//     server URL and accepts the default name) so the bar doesn't
//     show "localhost:9443 · localhost:9443".
//   * The current-user pill moved into the status-dot popover; users
//     already know who they are, and the inline chip ate horizontal
//     space the telemetry needs.
export default function StatusBar() {
    const [session, setSession] = useState(() => getSession());
    const [activeName, setActiveName] = useState(() => getActiveServer()?.name ?? null);
    const [info, setInfo] = useState<ServerInfo | null>(null);
    const [online, setOnline] = useState<"online" | "offline" | "error">("offline");
    const [lastPollAt, setLastPollAt] = useState<number | null>(null);
    const [lastPollMs, setLastPollMs] = useState<number | null>(null);
    const [lastError, setLastError] = useState<string | null>(null);
    const [memHistory, setMemHistory] = useState<number[]>([]);
    const [grtnHistory, setGrtnHistory] = useState<number[]>([]);
    const [cpuHistory, setCpuHistory] = useState<number[]>([]);
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
            setMemHistory([]);
            setGrtnHistory([]);
            setCpuHistory([]);
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
                if (fresh.mem_alloc_bytes !== undefined) {
                    setMemHistory((prev) => pushBounded(prev, fresh.mem_alloc_bytes!));
                }
                if (fresh.goroutines !== undefined) {
                    setGrtnHistory((prev) => pushBounded(prev, fresh.goroutines!));
                }
                if (fresh.cpu_percent !== undefined) {
                    setCpuHistory((prev) => pushBounded(prev, fresh.cpu_percent!));
                }
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

    // Hide the active-server profile chip when it equals the URL host
    // (the dev default of "localhost:9443" / "localhost:9443" was the
    // motivating regression). Showing a single label is a cleaner
    // signal than two identical ones separated by a bullet.
    const showActiveName = !!activeName && activeName !== serverHost;

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
                <span style={{ color: palette.border }}>·</span>
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
                            {showActiveName && (
                                <div>
                                    <span className="text-text-muted">Workspace: </span>
                                    <span className="text-text-primary">{activeName}</span>
                                </div>
                            )}
                            {info?.public_addr && (
                                <div data-testid="status-bar-ingress-popover">
                                    <span className="text-text-muted">Ingress: </span>
                                    <span className="text-text-primary">
                                        {info.public_addr}
                                    </span>
                                </div>
                            )}
                            {session?.user && (
                                <div data-testid="status-bar-user">
                                    <span className="text-text-muted">User: </span>
                                    <span className="text-text-primary">
                                        {session.user.username}
                                    </span>
                                </div>
                            )}
                        </div>
                    </PopoverContent>
                </Popover>
                {showActiveName && (
                    <span
                        style={{
                            color: palette.textSecondary,
                            fontWeight: 500,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                            minWidth: 0,
                        }}
                        title={activeName!}
                    >
                        {activeName}
                    </span>
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
                {lastPollMs !== null && (
                    <span
                        data-testid="status-bar-rtt"
                        title={`Last /info round-trip: ${lastPollMs} ms`}
                        style={{ color: palette.border }}
                    >
                        <span style={{ color: palette.border }}>·</span>{" "}
                        <Mono size={11} color={palette.textPrimary}>
                            {`${lastPollMs}ms`}
                        </Mono>
                    </span>
                )}
            </div>

            <div style={{ display: "flex", alignItems: "center", gap: space[3] }}>
                <TerminalsPill />
                <TransfersPill />
                <TransferThroughputPill />
                <RuntimePills
                    info={info}
                    memHistory={memHistory}
                    grtnHistory={grtnHistory}
                    cpuHistory={cpuHistory}
                />
                <CountPills info={info} />
                <Sep />
                <UtcClock />
                {session?.user && (
                    <>
                        <Sep />
                        <span
                            data-testid="status-bar-user-pill"
                            title={`Signed in as ${session.user.username} (${session.user.role})`}
                            style={{ color: palette.textPrimary, fontWeight: 500 }}
                        >
                            {session.user.username}
                        </span>
                    </>
                )}
                <VersionLinks info={info} />
            </div>
        </div>
    );
}

// pushBounded appends a sample to a ring buffer of up to HISTORY_SIZE
// entries. Returns a new array so React's setState sees the change.
function pushBounded(prev: number[], next: number): number[] {
    const out = [...prev, next];
    return out.length > HISTORY_SIZE ? out.slice(out.length - HISTORY_SIZE) : out;
}

// --- right-zone sub-components ----------------------------------------
// Each renders nothing (or "—") until the first /info response lands.
// They tick at 1 Hz alongside the parent's poll loop because the
// parent re-renders on every tick.

function RuntimePills({
    info,
    memHistory,
    grtnHistory,
    cpuHistory,
}: {
    info: ServerInfo | null;
    memHistory: number[];
    grtnHistory: number[];
    cpuHistory: number[];
}) {
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
                title="Resident memory (runtime.MemStats.Alloc) — last 60 s"
            >
                <span style={{ color: palette.textMuted }}>mem</span>
                <Mono size={11} color={palette.textPrimary}>
                    {formatBytes(info?.mem_alloc_bytes)}
                </Mono>
                <Sparkline
                    values={memHistory}
                    title="resident memory, last 60 s"
                    color={palette.info}
                />
            </Pill>
            <Sep />
            <Pill
                testid="status-bar-goroutines"
                title="Active goroutines (runtime.NumGoroutine) — last 60 s"
            >
                <span style={{ color: palette.textMuted }}>grtn</span>
                <Mono size={11} color={palette.textPrimary}>
                    {info?.goroutines ?? "—"}
                </Mono>
                <Sparkline
                    values={grtnHistory}
                    title="goroutines, last 60 s"
                    color={palette.success}
                />
            </Pill>
            <Sep />
            {/* Process CPU% — gopsutil's per-core normalised value
                (matches *nix `top`). Values >100% mean multi-core
                busy; the title spells that out so the chip doesn't
                read as a bug. */}
            <Pill
                testid="status-bar-cpu"
                title="Process CPU% — per-core normalised (>100% means multi-core busy across cores). Last 60 s."
            >
                <span style={{ color: palette.textMuted }}>cpu</span>
                <Mono size={11} color={palette.textPrimary}>
                    {info?.cpu_percent !== undefined
                        ? `${Math.round(info.cpu_percent)}%`
                        : "—"}
                </Mono>
                <Sparkline
                    values={cpuHistory}
                    title="cpu%, last 60 s"
                    color={palette.warning}
                />
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
                title="live hosts (last_seen within 60 s) / total enrolled hosts"
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
                title="live sessions (no disconnected_at) / total sessions ever"
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

// VersionLinks renders BOTH the server build version and the web
// build commit. The previous round dropped the web pill because
// __APP_VERSION__ (vite reads package.json) was always "0.0.0" in dev
// and looked broken. The replacement here uses __APP_COMMIT__ (set
// from the GIT_COMMIT env var by the Makefile / CI; falls back to
// "dev" for local builds), which is the actually-meaningful identifier
// for "what JS is the operator looking at?". The web pill renders as
// plain text — there's no GitHub release page for a commit hash, and
// "v0.0.0" was never a release anyway.
function VersionLinks({ info }: { info: ServerInfo | null }) {
    const repo = info?.git_repo || "WangYihang/Platypus";
    const serverVer = info?.version;
    const webCommit = __APP_COMMIT__;

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
            <Pill
                testid="status-bar-web-version"
                title={`web bundle commit ${webCommit}`}
            >
                <span style={{ color: palette.textMuted }}>web</span>
                <Mono size={11} color={palette.textSecondary}>
                    {webCommit.slice(0, 7)}
                </Mono>
            </Pill>
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
