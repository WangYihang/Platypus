import { useEffect, useRef, useState } from "react";
import { Router, Zap } from "lucide-react";

import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";
import { palette, radius, space } from "../layout/theme";
import { getSession, onActiveChange, onSessionChange } from "../lib/auth";
import { getActiveServer, onServersChange } from "../lib/servers";
import { ServerInfo, getServerInfo } from "../lib/api";
import Mono from "./Mono";
import StatusDot from "./StatusDot";

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
            try {
                const fresh = await getServerInfo();
                if (cancelled) return;
                setInfo(fresh);
                setOnline("online");
            } catch {
                if (cancelled) return;
                setOnline("error");
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

            <div style={{ display: "flex", alignItems: "center", gap: space[2] }}>
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
                {activeName && (
                    <>
                        <span style={{ color: palette.textSecondary, fontWeight: 500 }}>
                            {activeName}
                        </span>
                        <span style={{ color: palette.border }}>·</span>
                    </>
                )}
                <Mono size={11} color={palette.textMuted}>
                    {serverHost}
                </Mono>
                {session?.user && (
                    <>
                        <span style={{ color: palette.border }}>·</span>
                        <Mono size={11} color={palette.textMuted}>
                            {session.user.username}
                        </Mono>
                    </>
                )}
            </div>

            <div style={{ display: "flex", alignItems: "center", gap: space[3] }}>
                <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
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
