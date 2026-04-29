import { useEffect, useRef, useState } from "react";

import { EventsOff, EventsOn } from "@wails/runtime/runtime";
import { ServerInfo, getServerInfo } from "../lib/api";
import { getSession, onActiveChange, onSessionChange } from "../lib/auth";
import { getActiveServer, onServersChange } from "../lib/servers";

// 1 Hz so memory / goroutines / uptime tick like a proper telemetry strip.
// /info is small (one cheap COUNT roll-up + one ReadMemStats); cost is
// negligible.
const POLL_MS = 1_000;

// 60 samples × 1 Hz = the last minute of history per metric, matching
// the per-host topology sparklines.
const HISTORY_SIZE = 60;

export interface StatusBarMetrics {
    session: ReturnType<typeof getSession>;
    activeName: string | null;
    info: ServerInfo | null;
    online: "online" | "offline" | "error";
    lastPollAt: number | null;
    lastPollMs: number | null;
    lastError: string | null;
    memHistory: number[];
    grtnHistory: number[];
    cpuHistory: number[];
}

// useStatusBarMetrics owns the polling loop + Wails event subscription
// + history rings backing the StatusBar's right zone. Lifted out so
// the StatusBar component stays pure rendering.
export function useStatusBarMetrics(): StatusBarMetrics {
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
    // logout, server switch, rename.
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

        // Refresh immediately on client-churn notifications.
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

    return {
        session,
        activeName,
        info,
        online,
        lastPollAt,
        lastPollMs,
        lastError,
        memHistory,
        grtnHistory,
        cpuHistory,
    };
}

// Append a sample to a ring of up to HISTORY_SIZE entries. New array so
// React sees the change.
function pushBounded(prev: number[], next: number): number[] {
    const out = [...prev, next];
    return out.length > HISTORY_SIZE ? out.slice(out.length - HISTORY_SIZE) : out;
}
