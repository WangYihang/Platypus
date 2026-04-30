import { useQuery } from "@tanstack/react-query";

import AutoGrid from "../../components/AutoGrid";
import Card from "../../components/Card";
import DataList from "../../components/DataList";
import Mono from "../../components/Mono";
import RemoteAddr from "../../components/RemoteAddr";
import RefreshButton from "../../components/RefreshButton";
import StatusPill from "../../components/StatusPill";
import { palette, space } from "../../layout/theme";
import {
    Host,
    HostSysInfo,
    InstallPlatformsResponse,
    listInstallPlatforms,
} from "../../lib/api";
import { qk } from "../../lib/queryKeys";
import { fromNow } from "../../lib/time";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

import HardwareCard from "./HardwareCard";
import InfoKPIStrip from "./InfoKPIStrip";
import {
    formatBytes,
    formatPercent,
    renderLoadLine,
    renderMemoryLine,
    renderUptime,
} from "./format";
import {
    ApprovalStatusPill,
    BuildVersionValue,
    MachineTypePill,
} from "./pills";
import { UpgradeAgentButton } from "./UpgradeAgentButton";

interface Props {
    host: Host;
    sysInfo: HostSysInfo | null;
    sysInfoError: string | null;
    sysInfoLoading: boolean;
    onRefreshSysInfo: () => void;
}

// InfoTab renders the host detail Info tab. Three regions, top-down:
//
//   1. KPI strip (InfoKPIStrip): five live metrics at a glance.
//   2. Detail grid: Identity / System / Network / Hardware / Storage /
//      Logged-in users, in a CSS auto-fit grid. Each card prefers the
//      live SysInfo value and falls through to the DB-cached Host
//      column when the agent is offline.
//   3. Inline cached-fallback note when the live sysInfo fetch failed.
//
// Sub-card layout note: Identity / Network / Storage / Users are
// small enough to live inline here (50–80 lines each). System and
// Hardware are larger and live in dedicated files
// (SystemCard inline below since it's tightly coupled to the live /
// cached fallback logic; HardwareCard.tsx is its own module).
export default function InfoTab({
    host,
    sysInfo,
    sysInfoError,
    sysInfoLoading,
    onRefreshSysInfo,
}: Props) {
    // Prefer live sysInfo values; fall through to DB-cached Host.
    const kernel = sysInfo?.kernel_version || host.kernel_version;
    const platform = sysInfo?.platform || host.platform;
    const platformVersion = sysInfo?.platform_version || host.platform_version;
    const platformFamily = sysInfo?.platform_family || host.platform_family;
    const arch = sysInfo?.arch || host.arch;
    const cpuModel = sysInfo?.cpu_model || host.cpu_model;
    const numCPU = sysInfo?.num_cpu || host.num_cpu;
    const numCPUPhysical = sysInfo?.num_cpu_physical;
    const memTotal = sysInfo?.mem_total || host.mem_total_bytes;
    const currentUser = sysInfo?.current_user || host.current_user;
    const timezone = sysInfo?.timezone || host.timezone;
    const primaryIP = sysInfo?.primary_ip || host.primary_ip;
    const primaryMAC = sysInfo?.primary_mac || host.primary_mac;
    const bootTime = sysInfo?.boot_time_unix || host.boot_time_unix;
    const buildVersion = sysInfo?.build_version || host.build_version;
    const buildCommit = sysInfo?.build_commit || host.build_commit;
    const buildDate = sysInfo?.build_date || host.build_date;
    const protocolVersion =
        sysInfo?.protocol_version ?? host.protocol_version;

    // Latest channel head from the distributor manifest. Used to flag
    // outdated agents in the build-version row. Refresh-cadence is
    // generous (5min stale time) — manifest changes only when an
    // operator publishes a release, so polling tightly buys nothing.
    // 404 / distributor-disabled returns no data; the row falls back
    // to "no comparison available".
    const installPlatformsQuery = useQuery<InstallPlatformsResponse>({
        queryKey: qk.installPlatforms(),
        queryFn: () => listInstallPlatforms(),
        staleTime: 5 * 60 * 1000,
        gcTime: 10 * 60 * 1000,
        retry: false,
    });
    const latestVersion = installPlatformsQuery.data?.version || undefined;

    const liveBadge = sysInfoLoading ? (
        <StatusPill tone="neutral">refreshing…</StatusPill>
    ) : sysInfo ? (
        <StatusPill tone="success">live</StatusPill>
    ) : (
        <StatusPill tone="warning">cached</StatusPill>
    );

    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[4],
                maxWidth: 1100,
            }}
        >
            <InfoKPIStrip host={host} sysInfo={sysInfo} />
            <AutoGrid
                minSize={380}
                gap={3}
                style={{ alignItems: "start" }}
                data-testid="host-info-detail-grid"
            >
                <IdentityCard host={host} sysInfo={sysInfo} />

                <Card
                    header={
                        <span
                            style={{
                                display: "flex",
                                alignItems: "center",
                                gap: space[2],
                                justifyContent: "space-between",
                                width: "100%",
                            }}
                        >
                            <span
                                style={{
                                    display: "flex",
                                    alignItems: "center",
                                    gap: space[2],
                                }}
                            >
                                <span>System</span>
                                {liveBadge}
                            </span>
                            <RefreshButton
                                variant="ghost"
                                loading={sysInfoLoading}
                                onClick={onRefreshSysInfo}
                            />
                        </span>
                    }
                    padding={5}
                >
                    <DataList
                        items={[
                            {
                                label: "OS / arch",
                                value: (
                                    <span>
                                        {host.os || sysInfo?.os || "—"}
                                        {arch ? ` · ${arch}` : ""}
                                    </span>
                                ),
                            },
                            {
                                label: "platform",
                                value: (
                                    <span>
                                        {platform || "—"}
                                        {platformVersion ? ` ${platformVersion}` : ""}
                                        {platformFamily ? ` (${platformFamily})` : ""}
                                    </span>
                                ),
                            },
                            {
                                label: "kernel",
                                value: kernel ? <Mono>{kernel}</Mono> : "—",
                            },
                            {
                                label: "virtualization",
                                value: sysInfo?.virtualization ? (
                                    <Mono>{sysInfo.virtualization}</Mono>
                                ) : (
                                    "—"
                                ),
                            },
                            {
                                label: "CPU",
                                value: (
                                    <span>
                                        {cpuModel || "—"}
                                        {numCPU ? ` · ${numCPU}` : ""}
                                        {numCPU ? " logical" : ""}
                                        {numCPUPhysical
                                            ? ` / ${numCPUPhysical} physical`
                                            : ""}
                                        {sysInfo?.cpu_mhz
                                            ? ` · ${Math.round(sysInfo.cpu_mhz)} MHz`
                                            : ""}
                                    </span>
                                ),
                            },
                            {
                                label: "CPU usage",
                                value:
                                    sysInfo?.cpu_percent !== undefined
                                        ? `${sysInfo.cpu_percent.toFixed(1)} %`
                                        : "—",
                            },
                            {
                                label: "memory",
                                value: renderMemoryLine(
                                    sysInfo?.mem_used,
                                    memTotal,
                                    sysInfo?.mem_available,
                                ),
                            },
                            {
                                label: "swap",
                                value: renderMemoryLine(
                                    sysInfo?.swap_used,
                                    sysInfo?.swap_total,
                                ),
                            },
                            {
                                label: "load avg",
                                value: renderLoadLine(
                                    sysInfo?.load1,
                                    sysInfo?.load5,
                                    sysInfo?.load15,
                                ),
                            },
                            {
                                label: "uptime",
                                value: renderUptime(sysInfo?.uptime_seconds, bootTime),
                            },
                            { label: "timezone", value: timezone || "—" },
                            {
                                label: "current user",
                                value: currentUser ? <Mono>{currentUser}</Mono> : "—",
                            },
                            {
                                label: "processes",
                                value: sysInfo?.process_count
                                    ? String(sysInfo.process_count)
                                    : "—",
                            },
                            {
                                label: "build version",
                                value: (
                                    <span
                                        style={{
                                            display: "inline-flex",
                                            alignItems: "center",
                                            gap: space[3],
                                            flexWrap: "wrap",
                                        }}
                                    >
                                        <BuildVersionValue
                                            version={buildVersion}
                                            latest={latestVersion}
                                        />
                                        <UpgradeAgentButton
                                            projectID={host.project_id}
                                            hostID={host.id}
                                            currentVersion={buildVersion}
                                            latestVersion={latestVersion}
                                        />
                                    </span>
                                ),
                            },
                            {
                                label: "build commit",
                                value: buildCommit ? (
                                    <Mono size={11}>{buildCommit}</Mono>
                                ) : (
                                    "—"
                                ),
                            },
                            {
                                label: "build date",
                                value: buildDate ? <Mono>{buildDate}</Mono> : "—",
                            },
                            {
                                label: "protocol version",
                                value: protocolVersion ? String(protocolVersion) : "—",
                            },
                        ]}
                    />
                    {sysInfoError && !sysInfo && (
                        <div
                            style={{
                                marginTop: space[3],
                                fontSize: 12,
                                color: palette.textSecondary,
                            }}
                        >
                            Live metrics unavailable — showing last-known values. (
                            {sysInfoError})
                        </div>
                    )}
                </Card>

                <NetworkCard
                    sysInfo={sysInfo}
                    primaryIP={primaryIP}
                    primaryIPInfo={host.primary_ip_info}
                    primaryMAC={primaryMAC}
                    egressIP={host.egress_ip}
                    egressIPInfo={host.egress_ip_info}
                    publicIP={host.public_ip}
                    publicIPInfo={host.public_ip_info}
                />

                <HardwareCard host={host} sysInfo={sysInfo} />

                {sysInfo?.disks && sysInfo.disks.length > 0 && (
                    <StorageCard sysInfo={sysInfo} />
                )}

                {sysInfo?.users && sysInfo.users.length > 0 && (
                    <UsersCard sysInfo={sysInfo} />
                )}
            </AutoGrid>
        </div>
    );
}

function IdentityCard({
    host,
    sysInfo,
}: {
    host: Host;
    sysInfo: HostSysInfo | null;
}) {
    return (
        <Card header={<span>Identity</span>} padding={5}>
            <DataList
                items={[
                    {
                        label: "hostname",
                        value: host.hostname || sysInfo?.hostname || "—",
                    },
                    {
                        label: "machine type",
                        value: (
                            <MachineTypePill
                                type={sysInfo?.machine_type || host.machine_type}
                            />
                        ),
                    },
                    { label: "primary alias", value: host.primary_alias || "—" },
                    {
                        label: "agent id",
                        value: host.agent_id ? (
                            <Mono size={11}>{host.agent_id}</Mono>
                        ) : (
                            "—"
                        ),
                    },
                    {
                        label: "machine_id",
                        value: host.machine_id ? (
                            <Mono>{host.machine_id}</Mono>
                        ) : (
                            <StatusPill tone="warning">fingerprint fallback</StatusPill>
                        ),
                    },
                    {
                        label: "fingerprint",
                        value: <Mono size={11}>{host.fingerprint}</Mono>,
                    },
                    { label: "first seen", value: fromNow(host.first_seen_at) },
                    { label: "last seen", value: fromNow(host.last_seen_at) },
                    {
                        label: "approval",
                        value: <ApprovalStatusPill status={host.approval_status} />,
                    },
                ]}
            />
        </Card>
    );
}

function NetworkCard({
    sysInfo,
    primaryIP,
    primaryIPInfo,
    primaryMAC,
    egressIP,
    egressIPInfo,
    publicIP,
    publicIPInfo,
}: {
    sysInfo: HostSysInfo | null;
    primaryIP?: string;
    primaryIPInfo?: import("../../lib/api").RemoteIpInfo;
    primaryMAC?: string;
    egressIP?: string;
    egressIPInfo?: import("../../lib/api").RemoteIpInfo;
    publicIP?: string;
    publicIPInfo?: import("../../lib/api").RemoteIpInfo;
}) {
    // Prefer the server-side enriched public_ip cached on the host
    // row over the live sysInfo value — same address, but the host
    // version arrives with country/ISP already filled in. Falls back
    // to sysInfo + fetchInfo for the brief window before the first
    // SysInfo refresh persists.
    const effectivePublicIP = publicIP || sysInfo?.public_ip;
    const effectivePublicInfo =
        publicIPInfo && publicIPInfo.ip === effectivePublicIP ? publicIPInfo : undefined;
    return (
        <Card header="Network" padding={5}>
            <DataList
                items={[
                    {
                        label: "primary IP",
                        // Live sysInfo wins over the cached host.primary_ip,
                        // so primaryIPInfo only matches when the server-side
                        // cached value is the one being shown; otherwise we
                        // fall back to fetchInfo for ad-hoc enrichment.
                        value: primaryIP ? (
                            <RemoteAddr
                                addr={primaryIP}
                                info={
                                    primaryIPInfo && primaryIPInfo.ip === primaryIP
                                        ? primaryIPInfo
                                        : undefined
                                }
                                fetchInfo
                            />
                        ) : (
                            "—"
                        ),
                    },
                    {
                        label: "primary MAC",
                        value: primaryMAC ? <Mono>{primaryMAC}</Mono> : "—",
                    },
                    {
                        label: "egress IP",
                        // Server-derived: whatever the WS upgrade peered
                        // from on TCP. Diverges from primary IP for any
                        // agent behind NAT, and from public IP under mesh
                        // relay.
                        value: egressIP ? (
                            <RemoteAddr addr={egressIP} info={egressIPInfo} />
                        ) : (
                            "—"
                        ),
                    },
                    {
                        label: "default gateway",
                        value: sysInfo?.default_gateway ? (
                            <RemoteAddr addr={sysInfo.default_gateway} fetchInfo />
                        ) : (
                            "—"
                        ),
                    },
                    {
                        label: "public IP",
                        value: effectivePublicIP ? (
                            <RemoteAddr
                                addr={effectivePublicIP}
                                info={effectivePublicInfo}
                                fetchInfo={!effectivePublicInfo}
                            />
                        ) : (
                            "—"
                        ),
                    },
                ]}
            />
            {sysInfo?.interfaces && sysInfo.interfaces.length > 0 && (
                <div style={{ marginTop: space[4] }}>
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[160px]">interface</TableHead>
                                <TableHead>MAC</TableHead>
                                <TableHead>addresses</TableHead>
                                <TableHead className="w-[100px]">state</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {sysInfo.interfaces.map((ifi) => (
                                <TableRow key={ifi.name}>
                                    <TableCell>
                                        <Mono>{ifi.name}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        {ifi.mac ? (
                                            <Mono size={11}>{ifi.mac}</Mono>
                                        ) : (
                                            "—"
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        {ifi.addrs && ifi.addrs.length > 0 ? (
                                            <Mono size={11}>{ifi.addrs.join(", ")}</Mono>
                                        ) : (
                                            "—"
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        {ifi.is_up ? (
                                            <StatusPill tone="success">up</StatusPill>
                                        ) : (
                                            <StatusPill tone="neutral">down</StatusPill>
                                        )}
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </div>
            )}
        </Card>
    );
}

function StorageCard({ sysInfo }: { sysInfo: HostSysInfo }) {
    return (
        <Card header="Storage" padding={5}>
            <Table>
                <TableHeader>
                    <TableRow>
                        <TableHead>mount</TableHead>
                        <TableHead>device</TableHead>
                        <TableHead className="w-[90px]">fs</TableHead>
                        <TableHead className="w-[120px]">used</TableHead>
                        <TableHead className="w-[120px]">total</TableHead>
                        <TableHead className="w-[70px]">%</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {(sysInfo.disks ?? []).map((d, i) => (
                        <TableRow key={`${d.mountpoint}-${i}`}>
                            <TableCell>
                                <Mono>{d.mountpoint}</Mono>
                            </TableCell>
                            <TableCell>
                                <Mono size={11}>{d.device || "—"}</Mono>
                            </TableCell>
                            <TableCell>{d.fstype || "—"}</TableCell>
                            <TableCell>{formatBytes(d.used_bytes)}</TableCell>
                            <TableCell>{formatBytes(d.total_bytes)}</TableCell>
                            <TableCell>
                                {formatPercent(d.used_bytes, d.total_bytes)}
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>
        </Card>
    );
}

function UsersCard({ sysInfo }: { sysInfo: HostSysInfo }) {
    return (
        <Card header="Logged-in users" padding={5} style={{ gridColumn: "1 / -1" }}>
            <Table>
                <TableHeader>
                    <TableRow>
                        <TableHead className="w-[140px]">user</TableHead>
                        <TableHead className="w-[140px]">terminal</TableHead>
                        <TableHead>from</TableHead>
                        <TableHead className="w-[160px]">since</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {(sysInfo.users ?? []).map((u, i) => (
                        <TableRow key={`${u.user}-${u.terminal}-${i}`}>
                            <TableCell>
                                <Mono>{u.user || "—"}</Mono>
                            </TableCell>
                            <TableCell>
                                <Mono size={11}>{u.terminal || "—"}</Mono>
                            </TableCell>
                            <TableCell>
                                {u.host ? <Mono size={11}>{u.host}</Mono> : "—"}
                            </TableCell>
                            <TableCell className="text-text-secondary">
                                {u.started_at
                                    ? fromNow(
                                          new Date(u.started_at * 1000).toISOString(),
                                      )
                                    : "—"}
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>
        </Card>
    );
}
