import { useEffect, useState } from "react";
import { Empty, Tag } from "antd";

import Terminal from "../Terminal";
import { SessionRow } from "../../lib/api";
import { palette } from "../../layout/theme";

interface Props {
    // Live (disconnected_at === null) sessions for the host. Parent
    // feeds this in from HostView's already-fetched sessions list.
    liveSessions: SessionRow[];
}

// TerminalTab embeds the double-mode xterm (pages/Terminal.tsx) inside
// HostView. When the host has multiple live sessions (same machine
// reconnected through different listeners, or multiple agents), the
// user picks one via a chip row; the selected session drives the
// xterm instance below.
//
// Parent re-renders this with a fresh liveSessions array when a session
// opens or closes (driven by session.opened / session.closed events).
// If the currently-picked session disappears, we fall back to the
// first remaining one.
export default function TerminalTab({ liveSessions }: Props) {
    const [picked, setPicked] = useState<string | null>(null);

    // Ensure `picked` always points at a live session — fall back to
    // the first one if the previous pick just closed.
    useEffect(() => {
        if (liveSessions.length === 0) {
            setPicked(null);
            return;
        }
        if (!picked || !liveSessions.some((s) => s.id === picked)) {
            setPicked(liveSessions[0].id);
        }
    }, [liveSessions, picked]);

    if (liveSessions.length === 0) {
        return (
            <div style={{ padding: 32 }}>
                <Empty description="No live sessions — waiting for the agent to reconnect." />
            </div>
        );
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {liveSessions.length > 1 && (
                <div
                    style={{
                        display: "flex",
                        gap: 6,
                        padding: "8px 0 12px",
                        flexWrap: "wrap",
                    }}
                >
                    {liveSessions.map((s) => {
                        const selected = s.id === picked;
                        return (
                            <Tag
                                key={s.id}
                                color={selected ? "blue" : undefined}
                                style={{
                                    cursor: "pointer",
                                    padding: "3px 10px",
                                    fontSize: 12,
                                    borderRadius: 4,
                                }}
                                onClick={() => setPicked(s.id)}
                            >
                                <span
                                    style={{
                                        color: selected
                                            ? undefined
                                            : palette.textPrimary,
                                        fontFamily: "Menlo, Consolas, monospace",
                                    }}
                                >
                                    {s.id.slice(0, 12)}
                                </span>
                                {s.user && (
                                    <span
                                        style={{
                                            marginLeft: 6,
                                            color: palette.textSecondary,
                                            fontSize: 11,
                                        }}
                                    >
                                        · {s.user}
                                    </span>
                                )}
                            </Tag>
                        );
                    })}
                </div>
            )}
            <div style={{ flex: 1, minHeight: 0 }}>
                {picked && <Terminal key={picked} sessionHash={picked} />}
            </div>
        </div>
    );
}
