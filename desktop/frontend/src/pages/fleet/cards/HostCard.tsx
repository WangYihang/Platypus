import { Cpu } from "lucide-react";

import Mono from "../../../components/Mono";
import RemoteAddr from "../../../components/RemoteAddr";
import StatusDot from "../../../components/StatusDot";
import StatusPill from "../../../components/StatusPill";
import { palette, radius, space } from "../../../layout/theme";
import { Host } from "../../../lib/api";
import { fromNow, isOnline } from "../../../lib/time";

import ApprovalActions from "./ApprovalActions";
import Dim from "./Dim";
import { MachineTypeIcon } from "./machineIcons";
import { formatMem, osLabel } from "./format";

interface Props {
    host: Host;
    onOpen: () => void;
    approving: boolean;
    rejecting: boolean;
    onApprove: () => void;
    onReject: () => void;
}

// HostCard is one tile in the Fleet card grid. Three visual modes,
// driven entirely by `host.approval_status`:
//
//   approved → standard card
//   pending  → standard card + warning pill + inline Approve/Reject
//              (replaces the round-trip through /fleet/approvals for
//              the common single-host case)
//   rejected → standard card with a muted "rejected" pill
//
// The whole card is a button that navigates to the host detail; the
// approval action row inside stops propagation so clicks there don't
// also navigate.
export default function HostCard({
    host,
    onOpen,
    approving,
    rejecting,
    onApprove,
    onReject,
}: Props) {
    const online = isOnline(host.last_seen_at);
    const primary =
        host.primary_alias ||
        host.hostname ||
        host.machine_id?.slice(0, 8) ||
        "unknown";
    const pending = host.approval_status === "pending";
    const rejected = host.approval_status === "rejected";

    return (
        <button
            type="button"
            onClick={onOpen}
            data-testid="fleet-card"
            data-online={online ? "true" : "false"}
            data-approval={host.approval_status}
            style={{
                textAlign: "left",
                background: palette.surface,
                border: `1px solid ${pending ? palette.warning : palette.border}`,
                borderRadius: radius.md,
                padding: `${space[4]}px ${space[4]}px ${space[3]}px`,
                cursor: "pointer",
                display: "flex",
                flexDirection: "column",
                gap: space[3],
                transition: "border-color 120ms ease, background 120ms ease",
                color: palette.textPrimary,
                fontFamily: "var(--font-geist-mono)",
            }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    minWidth: 0,
                }}
            >
                <span style={{ flexShrink: 0 }}>
                    <MachineTypeIcon type={host.machine_type} />
                </span>
                <StatusDot status={online ? "online" : "offline"} />
                <span
                    style={{
                        fontWeight: 600,
                        fontSize: 14,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        flex: 1,
                        minWidth: 0,
                    }}
                    title={primary}
                >
                    {primary}
                </span>
                {pending && <StatusPill tone="warning">pending</StatusPill>}
                {rejected && <StatusPill tone="neutral">rejected</StatusPill>}
            </div>

            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "auto 1fr",
                    rowGap: 4,
                    columnGap: space[2],
                    fontSize: 12,
                    color: palette.textSecondary,
                }}
            >
                <span style={{ color: palette.textMuted }}>OS</span>
                <span
                    style={{
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                >
                    {osLabel(host) ?? <Dim>—</Dim>}
                </span>
                <span style={{ color: palette.textMuted }}>Arch</span>
                <span>
                    {host.arch ? <Mono>{host.arch}</Mono> : <Dim>—</Dim>}
                </span>
                <span style={{ color: palette.textMuted }}>IP</span>
                <span>
                    {host.primary_ip ? (
                        <RemoteAddr addr={host.primary_ip} info={host.primary_ip_info} />
                    ) : (
                        <Dim>—</Dim>
                    )}
                </span>
                <span style={{ color: palette.textMuted }}>Egress</span>
                <span>
                    {host.egress_ip ? (
                        <RemoteAddr addr={host.egress_ip} info={host.egress_ip_info} />
                    ) : (
                        <Dim>—</Dim>
                    )}
                </span>
                <span style={{ color: palette.textMuted }}>Hardware</span>
                <span
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: space[2],
                    }}
                >
                    <span
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 4,
                        }}
                    >
                        <Cpu className="size-3" />
                        {host.num_cpu ? `${host.num_cpu}×` : "—"}
                    </span>
                    <span style={{ color: palette.border }}>·</span>
                    <span>{formatMem(host.mem_total_bytes)}</span>
                </span>
            </div>

            <div
                style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    fontSize: 11,
                    color: palette.textMuted,
                    borderTop: `1px solid ${palette.border}`,
                    paddingTop: space[2],
                }}
            >
                <span>
                    {host.machine_id ? (
                        <Mono size={11}>{host.machine_id.slice(0, 12)}…</Mono>
                    ) : (
                        "fp pending"
                    )}
                </span>
                <span>{fromNow(host.last_seen_at)}</span>
            </div>

            {pending && (
                <ApprovalActions
                    host={host}
                    approving={approving}
                    rejecting={rejecting}
                    onApprove={onApprove}
                    onReject={onReject}
                />
            )}
        </button>
    );
}
