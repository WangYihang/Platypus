import { useEffect, useRef, useState } from "react";
import { Router, Zap } from "lucide-react";

import { EventsOff, EventsOn } from "@wails/runtime/runtime";
import { palette, radius, space } from "../layout/theme";
import { getSession, onActiveChange, onSessionChange } from "../lib/auth";
import { getActiveServer, onServersChange } from "../lib/servers";
import { ServerInfo, getServerInfo } from "../lib/api";
import TerminalsPill from "../terminal/TerminalsPill";
import Mono from "./Mono";
import StatusDot from "./StatusDot";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

// Refresh cadence: 10s is enough for a status bar — listener/session
// churn isn't so hot that a tighter interval would noticeably help.
const POLL_MS = 10_000;

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
                <span
                    data-testid="status-bar-ingress"
                    style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 4,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        minWidth: 0,
                    }}
                    title={info?.public_addr || ""}
                >
                    <Router className="size-3" />
                    <span>Ingress</span>
                    <Mono size={11} color={palette.textPrimary}>
                        {info?.public_addr || "—"}
                    </Mono>
                </span>
                <span style={{ color: palette.border }}>·</span>
                <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    <Zap className="size-3" />
                    <span>Sessions</span>
                    <Mono size={11} color={palette.textPrimary}>
                        {info?.session_count ?? "—"}
                    </Mono>
                </span>
                <span style={{ color: palette.border }}>·</span>
                <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    <span>Server</span>
                    <Mono size={11} color={palette.textSecondary}>
                        {info ? `v${info.version}` : "—"}
                    </Mono>
                </span>
            </div>
        </div>
    );
}
