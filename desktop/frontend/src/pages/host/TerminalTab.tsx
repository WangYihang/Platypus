import { Empty, Tag } from "antd";

import Terminal from "../Terminal";
import { SessionRow } from "../../lib/api";
import { palette } from "../../layout/theme";

interface Props {
    // Live (disconnected_at === null) sessions for the host. Parent
    // feeds this in from HostView's already-fetched sessions list.
    liveSessions: SessionRow[];
    // Currently-picked session. Managed by HostView so the Files and
    // Tunnels tabs see the same pick.
    picked: string | null;
    onPick: (sessionHash: string) => void;
}

// TerminalTab embeds the double-mode xterm (pages/Terminal.tsx) inside
// HostView. When the host has multiple live sessions (same machine
// reconnected through different listeners, or multiple agents), the
// user picks one via a chip row; the selected session drives the
// xterm instance below.
//
// The picked session lives one level up in HostView so Files / Tunnels
// tabs operate on the same connection. Parent handles the "picked just
// closed, pick the next one" fallback in a single useEffect.
export default function TerminalTab({ liveSessions, picked, onPick }: Props) {
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
                                onClick={() => onPick(s.id)}
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
