// useTopology — the page-level controller hook.
//
// Responsibilities:
//   1. Fetch the initial TopologySnapshot from the REST API.
//   2. Subscribe to /notify and reduce topology.* events into the
//      same shape.
//   3. For link_stats events, compute per-edge instantaneous rate by
//      diffing against the previous observation (wrap-tolerant).
//   4. Keep a short client-side ring buffer of recent CPU / memory
//      samples per machine so the detail panel can render a
//      sparkline without hitting the history API.

import { useEffect, useMemo, useRef, useState } from "react";

import {
    TopologySnapshot,
    TopologyMachine,
    TopologyMeshNodeRef,
    TopologyLink,
    fetchTopologySnapshot,
} from "../../lib/api";
import {
    NotifyEvent,
    onNotify,
    TopologyLinkStatsPayload,
    TopologyMachineStatsPayload,
    TopologyLinkUpPayload,
    TopologyLinkDownPayload,
} from "../../lib/notify";

// RingSize is how many samples we retain per entity for sparkline
// rendering. 60 * 1 Hz = 60 seconds of history, which matches the
// sparklines in the detail panels.
const RingSize = 60;

export interface TopologyState {
    snapshot: TopologySnapshot | null;
    // Derived, per-edge rates: bytes/s and msgs/s, keyed by canonical
    // "a|b" pair. Computed from successive link_stats frames.
    linkRates: Map<string, LinkRate>;
    machineHistory: Map<string, MachineSeries>;
    loading: boolean;
    error: string | null;
}

export interface LinkRate {
    bytesInPerSec: number;
    bytesOutPerSec: number;
    msgsInPerSec: number;
    msgsOutPerSec: number;
    rttMs: number;
    at: number; // unix ms of the most recent observation
}

export interface MachineSeries {
    // Most recent 60 samples, oldest first.
    cpu: Array<{ t: number; v: number }>;
    mem: Array<{ t: number; v: number }>;
}

function edgeKey(a: string, b: string): string {
    return a < b ? `${a}|${b}` : `${b}|${a}`;
}

export function useTopology(projectId: string): TopologyState {
    const [snapshot, setSnapshot] = useState<TopologySnapshot | null>(null);
    const [loading, setLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);
    const [linkRates, setLinkRates] = useState<Map<string, LinkRate>>(() => new Map());
    const [machineHistory, setMachineHistory] = useState<Map<string, MachineSeries>>(() => new Map());

    // Per-edge last cumulative observation. Ref so updating doesn't
    // trigger a rerender — we only push the derived rate to state.
    const prevLinkSamples = useRef<
        Map<string, { bytesIn: number; bytesOut: number; msgsIn: number; msgsOut: number; at: number }>
    >(new Map());

    // Initial fetch.
    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        setError(null);
        fetchTopologySnapshot(projectId)
            .then((snap) => {
                if (cancelled) return;
                setSnapshot(snap);
                setLoading(false);
            })
            .catch((e) => {
                if (cancelled) return;
                setError(String(e));
                setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [projectId]);

    // Subscribe to topology.* notify events.
    useEffect(() => {
        const unsubs: Array<() => void> = [];

        unsubs.push(
            onNotify(NotifyEvent.TopologyLinkStats, (data) => {
                const payload = data as TopologyLinkStatsPayload;
                if (payload.project_id !== projectId) return;
                const now = Date.now();
                setLinkRates((prev) => {
                    const next = new Map(prev);
                    for (const l of payload.links) {
                        const key = edgeKey(l.a, l.b);
                        const before = prevLinkSamples.current.get(key);
                        prevLinkSamples.current.set(key, {
                            bytesIn: l.bytes_in,
                            bytesOut: l.bytes_out,
                            msgsIn: l.msgs_in,
                            msgsOut: l.msgs_out,
                            at: now,
                        });
                        if (!before) {
                            next.set(key, {
                                bytesInPerSec: 0,
                                bytesOutPerSec: 0,
                                msgsInPerSec: 0,
                                msgsOutPerSec: 0,
                                rttMs: l.rtt_ns / 1e6,
                                at: now,
                            });
                            continue;
                        }
                        const dt = Math.max(1, (now - before.at) / 1000);
                        // Wrap-tolerant: a decrease resets the baseline to the
                        // current sample. Avoids wildly negative rates on
                        // agent restart.
                        const delta = (cur: number, prev: number) =>
                            cur >= prev ? (cur - prev) / dt : 0;
                        next.set(key, {
                            bytesInPerSec: delta(l.bytes_in, before.bytesIn),
                            bytesOutPerSec: delta(l.bytes_out, before.bytesOut),
                            msgsInPerSec: delta(l.msgs_in, before.msgsIn),
                            msgsOutPerSec: delta(l.msgs_out, before.msgsOut),
                            rttMs: l.rtt_ns / 1e6,
                            at: now,
                        });
                    }
                    // Also push updated cumulative counters onto the
                    // snapshot's link rows so tooltips stay fresh.
                    return next;
                });
                // Merge cumulative counters back into snapshot.links.
                setSnapshot((prev) => {
                    if (!prev) return prev;
                    const byKey: Record<string, TopologyLink> = {};
                    for (const l of prev.links) byKey[edgeKey(l.a, l.b)] = l;
                    for (const l of payload.links) {
                        const k = edgeKey(l.a, l.b);
                        const existing = byKey[k];
                        if (existing) {
                            byKey[k] = {
                                ...existing,
                                bytes_in: l.bytes_in,
                                bytes_out: l.bytes_out,
                                msgs_in: l.msgs_in,
                                msgs_out: l.msgs_out,
                                rtt_ns: l.rtt_ns,
                                up: true,
                            };
                        }
                    }
                    return { ...prev, links: Object.values(byKey) };
                });
            }),
        );

        unsubs.push(
            onNotify(NotifyEvent.TopologyMachineStats, (data) => {
                const payload = data as TopologyMachineStatsPayload;
                if (payload.project_id !== projectId) return;
                const now = Date.now();
                setMachineHistory((prev) => {
                    const next = new Map(prev);
                    const s = next.get(payload.host_id) ?? { cpu: [], mem: [] };
                    const append = (arr: Array<{ t: number; v: number }>, v: number) => {
                        const out = [...arr, { t: now, v }];
                        return out.length > RingSize ? out.slice(out.length - RingSize) : out;
                    };
                    next.set(payload.host_id, {
                        cpu: append(s.cpu, payload.cpu_percent),
                        mem: append(s.mem, payload.mem_percent),
                    });
                    return next;
                });
                // Stamp the latest sysinfo / percentages onto the
                // snapshot machine so detail panels reflect them
                // without waiting for the next snapshot refresh.
                setSnapshot((prev) => {
                    if (!prev) return prev;
                    const machines = prev.machines.map((m) => {
                        if (m.host_id !== payload.host_id) return m;
                        return {
                            ...m,
                            sys_info: {
                                ...(m.sys_info ?? {}),
                                cpu_percent: payload.cpu_percent,
                                mem_percent: payload.mem_percent,
                                ...(payload.sys_info ?? {}),
                                sampled_at_unix: payload.sampled_at,
                            },
                        } satisfies TopologyMachine;
                    });
                    return { ...prev, machines };
                });
            }),
        );

        unsubs.push(
            onNotify(NotifyEvent.TopologyLinkUp, (data) => {
                const payload = data as TopologyLinkUpPayload;
                if (payload.project_id !== projectId) return;
                // Refresh from REST — link_up may add a mesh node we
                // haven't seen yet. Cheap: same endpoint used on mount.
                fetchTopologySnapshot(projectId).then(setSnapshot).catch(() => {});
            }),
        );

        unsubs.push(
            onNotify(NotifyEvent.TopologyLinkDown, (data) => {
                const payload = data as TopologyLinkDownPayload;
                if (payload.project_id !== projectId) return;
                setSnapshot((prev) => {
                    if (!prev) return prev;
                    return {
                        ...prev,
                        links: prev.links.map((l) =>
                            l.a === payload.peer || l.b === payload.peer
                                ? { ...l, up: false }
                                : l,
                        ),
                    };
                });
            }),
        );

        return () => {
            for (const u of unsubs) u();
        };
    }, [projectId]);

    const state = useMemo<TopologyState>(
        () => ({
            snapshot,
            linkRates,
            machineHistory,
            loading,
            error,
        }),
        [snapshot, linkRates, machineHistory, loading, error],
    );
    return state;
}

// Re-export for convenience.
export type { TopologyMachine, TopologyMeshNodeRef, TopologyLink };
