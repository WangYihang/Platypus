import { Tooltip } from "antd";

import { Host } from "../lib/api";
import { isOnline } from "../lib/time";
import { palette } from "./theme";

interface Props {
    host: Host;
    selected: boolean;
    onSelect: () => void;
}

// HostRow renders one host in the sidebar. Presence dot colour reflects
// the host's last_seen_at (green within 60s, grey otherwise) — the
// threshold is centralised in lib/time.ONLINE_WINDOW_MS. Title line is
// hostname (or primary_alias, falling back to "unknown"); subtitle is
// the OS so a big fleet remains scannable without hovering.
export default function HostRow({ host, selected, onSelect }: Props) {
    const online = isOnline(host.last_seen_at);
    const primary =
        host.primary_alias || host.hostname || host.machine_id?.slice(0, 8) || "unknown host";
    const secondary = [host.os, host.fingerprint_fallback ? "fp-fallback" : ""]
        .filter(Boolean)
        .join(" · ");

    return (
        <div
            role="button"
            tabIndex={0}
            onClick={onSelect}
            onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") onSelect();
            }}
            style={{
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "6px 12px 6px 28px",
                cursor: "pointer",
                color: selected ? palette.textPrimary : palette.textSecondary,
                background: selected ? palette.main : "transparent",
                borderLeft: selected ? `2px solid ${palette.accent}` : "2px solid transparent",
                fontSize: 13,
                userSelect: "none",
            }}
        >
            <Tooltip title={online ? "Online" : "Last seen some time ago"}>
                <span
                    style={{
                        width: 8,
                        height: 8,
                        borderRadius: "50%",
                        background: online ? palette.success : palette.textSecondary,
                        flexShrink: 0,
                        opacity: online ? 1 : 0.5,
                    }}
                />
            </Tooltip>
            <div
                style={{
                    display: "flex",
                    flexDirection: "column",
                    overflow: "hidden",
                    flex: 1,
                    minWidth: 0,
                }}
            >
                <span
                    style={{
                        color: selected ? palette.textPrimary : palette.textPrimary,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        fontWeight: selected ? 600 : 400,
                    }}
                >
                    {primary}
                </span>
                {secondary && (
                    <span
                        style={{
                            color: palette.textSecondary,
                            fontSize: 11,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                        }}
                    >
                        {secondary}
                    </span>
                )}
            </div>
        </div>
    );
}
