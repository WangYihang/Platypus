import { ReactNode } from "react";
import { ArrowLeft } from "lucide-react";
import { Link } from "react-router-dom";

import StatusDot from "../../components/StatusDot";
import { palette, space } from "../../layout/theme";
import { Host } from "../../lib/api";
import { isOnline } from "../../lib/time";

interface Props {
    project: { slug: string };
    host: Host;
    actions?: ReactNode;
}

// HostHeaderBar is the top strip of the right pane in the master-detail
// HostView. Plays double duty:
//   · Wide viewport: identity strip beside the master rail.
//   · Narrow viewport (<960px): the rail hides; the "← Hosts" link
//     becomes the back button to /fleet/hosts. We render it
//     unconditionally so wide-screen muscle memory still has the
//     escape hatch.
const HEADER_PX = 40;

export default function HostHeaderBar({ project, host, actions }: Props) {
    const primary =
        host.primary_alias || host.hostname || host.machine_id?.slice(0, 8) || "unknown";
    const online = isOnline(host.last_seen_at);

    return (
        <div
            data-testid="host-header-bar"
            style={{
                flexShrink: 0,
                height: HEADER_PX,
                display: "flex",
                alignItems: "center",
                gap: space[3],
                padding: `0 ${space[3]}px`,
                borderBottom: `1px solid ${palette.border}`,
                background: palette.rail,
                fontSize: 12,
            }}
        >
            <Link
                to={`/projects/${project.slug}/fleet/hosts`}
                aria-label="Back to hosts"
                title="Back to hosts"
                style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 4,
                    color: palette.textMuted,
                    textDecoration: "none",
                    padding: "2px 4px",
                    borderRadius: 4,
                    fontSize: 12,
                }}
            >
                <ArrowLeft className="size-3.5" />
                <span>Hosts</span>
            </Link>
            <span
                aria-hidden
                style={{ width: 1, height: 16, background: palette.border, flexShrink: 0 }}
            />
            <span style={{ display: "inline-flex", alignItems: "center", gap: space[2], minWidth: 0 }}>
                <StatusDot status={online ? "online" : "offline"} />
                <span
                    style={{
                        fontWeight: 600,
                        color: palette.textPrimary,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        maxWidth: 280,
                    }}
                >
                    {primary}
                </span>
            </span>
            {host.os && (
                <span style={pillStyle} title="Reported OS">
                    {host.os}
                </span>
            )}
            {host.primary_ip && (
                <span style={pillStyle} title="Primary IP">
                    {host.primary_ip}
                </span>
            )}
            {host.fingerprint_fallback && (
                <span
                    style={{ ...pillStyle, color: palette.warning }}
                    title="Agent didn't report a stable platform machine_id"
                >
                    fp-fallback
                </span>
            )}
            <span style={{ flex: 1 }} />
            {actions}
        </div>
    );
}

const pillStyle: React.CSSProperties = {
    padding: "1px 6px",
    fontSize: 11,
    color: palette.textMuted,
    background: palette.surface,
    border: `1px solid ${palette.border}`,
    borderRadius: 4,
    fontFamily: "var(--font-geist-mono)",
    whiteSpace: "nowrap",
};
