import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import { font, palette, radius, space } from "../../layout/theme";
import { SessionRow } from "../../lib/api";
import Terminal from "../Terminal";

interface Props {
    // Live (disconnected_at === null) sessions for the host. Parent
    // feeds this in from HostView's already-fetched sessions list.
    liveSessions: SessionRow[];
    // Currently-picked session. Managed by HostView so the Files and
    // Tunnels tabs see the same pick.
    picked: string | null;
    onPick: (sessionHash: string) => void;
}

// TerminalTab embeds the xterm wrapper inside HostView. When the host
// has multiple live sessions (same machine reconnected through different
// listeners, or multiple agents), the user picks one via a Vercel-style
// chip row; the selected session drives the xterm instance below.
export default function TerminalTab({ liveSessions, picked, onPick }: Props) {
    if (liveSessions.length === 0) {
        return (
            <EmptyState
                title="No live session"
                description="Waiting for the agent to reconnect to a listener."
            />
        );
    }

    const showPicker = liveSessions.length > 1;

    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                height: "100%",
                gap: space[3],
            }}
        >
            {showPicker && (
                <div
                    style={{
                        display: "flex",
                        flexWrap: "wrap",
                        gap: space[2],
                    }}
                >
                    {liveSessions.map((s) => {
                        const selected = s.id === picked;
                        return (
                            <button
                                key={s.id}
                                onClick={() => onPick(s.id)}
                                style={{
                                    display: "inline-flex",
                                    alignItems: "center",
                                    gap: space[2],
                                    padding: `4px ${space[3]}px`,
                                    background: selected
                                        ? palette.surfaceHover
                                        : "transparent",
                                    border: `1px solid ${
                                        selected ? palette.textPrimary : palette.border
                                    }`,
                                    borderRadius: radius.md,
                                    color: palette.textPrimary,
                                    fontFamily: font.mono,
                                    fontSize: 12,
                                    fontWeight: 500,
                                    cursor: "pointer",
                                    transition: "border-color 120ms ease",
                                }}
                            >
                                <Mono size={12}>{s.id.slice(0, 12)}</Mono>
                                {s.user && (
                                    <span
                                        style={{
                                            color: palette.textSecondary,
                                            fontFamily: font.sans,
                                            fontSize: 11,
                                        }}
                                    >
                                        · {s.user}
                                    </span>
                                )}
                            </button>
                        );
                    })}
                </div>
            )}
            <div
                style={{
                    flex: 1,
                    minHeight: 320,
                    border: `1px solid ${palette.border}`,
                    borderRadius: radius.md,
                    overflow: "hidden",
                    background: palette.main,
                }}
            >
                {picked && <Terminal key={picked} sessionHash={picked} />}
            </div>
        </div>
    );
}
