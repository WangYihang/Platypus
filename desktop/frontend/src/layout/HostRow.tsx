import { Tooltip } from "antd";

import StatusDot from "../components/StatusDot";
import { Host } from "../lib/api";
import { isOnline } from "../lib/time";
import { palette, space } from "./theme";

interface Props {
    host: Host;
    selected: boolean;
    onSelect: () => void;
}

// HostRow renders one host in the sidebar. Online state via the shared
// StatusDot primitive (8px halo'd dot). Title line is hostname/alias;
// subtitle is OS so a big fleet remains scannable without hovering.
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
                gap: space[2],
                padding: `${space[2]}px ${space[3]}px ${space[2]}px 28px`,
                cursor: "pointer",
                color: selected ? palette.textPrimary : palette.textSecondary,
                background: selected ? palette.surfaceHover : "transparent",
                borderLeft: selected
                    ? `2px solid ${palette.textPrimary}`
                    : "2px solid transparent",
                fontSize: 13,
                userSelect: "none",
            }}
        >
            <Tooltip title={online ? "Online" : "Last seen some time ago"}>
                <span style={{ display: "inline-flex" }}>
                    <StatusDot status={online ? "online" : "offline"} />
                </span>
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
                        color: palette.textPrimary,
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
                            color: palette.textMuted,
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
