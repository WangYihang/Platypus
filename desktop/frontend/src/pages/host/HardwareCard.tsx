import Card from "../../components/Card";
import DataList from "../../components/DataList";
import Mono from "../../components/Mono";
import { palette, space } from "../../layout/theme";
import { Host, HostSysInfo } from "../../lib/api";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

import { formatBytes } from "./format";
import { MachineTypePill } from "./pills";

interface Props {
    host: Host;
    sysInfo: HostSysInfo | null;
}

// HardwareCard surfaces the chassis / product / BIOS identity plus
// the GPU list. Everything is optional — if the agent had no way to
// read DMI and ghw returned nothing we still render the card with
// "—" placeholders so operators can tell the probe ran but found
// nothing rather than "was this even collected?".
//
// When the agent reports structured GPUs we render them as a small
// table. For older agents (or DBs migrated from the freeform field)
// we fall through to the legacy `host.gpu_summary` blurb.
export default function HardwareCard({ host, sysInfo }: Props) {
    const productVendor = sysInfo?.product_vendor || host.product_vendor;
    const productName = sysInfo?.product_name || host.product_name;
    const biosVendor = sysInfo?.bios_vendor || host.bios_vendor;
    const biosVersion = sysInfo?.bios_version || host.bios_version;
    const chassis = sysInfo?.chassis_type || host.chassis_type;
    const containerRuntime = sysInfo?.container_runtime;
    const gpus = sysInfo?.gpus || [];

    return (
        <Card header="Hardware" padding={5}>
            <DataList
                items={[
                    {
                        label: "machine type",
                        value: (
                            <MachineTypePill
                                type={sysInfo?.machine_type || host.machine_type}
                            />
                        ),
                    },
                    ...(containerRuntime
                        ? [
                              {
                                  label: "container runtime",
                                  value: <Mono>{containerRuntime}</Mono>,
                              },
                          ]
                        : []),
                    {
                        label: "chassis",
                        value: chassis ? <Mono>{chassis}</Mono> : "—",
                    },
                    {
                        label: "product",
                        value: (
                            <span>
                                {productVendor || "—"}
                                {productName ? ` · ${productName}` : ""}
                            </span>
                        ),
                    },
                    {
                        label: "BIOS",
                        value: (
                            <span>
                                {biosVendor || "—"}
                                {biosVersion ? ` · ${biosVersion}` : ""}
                            </span>
                        ),
                    },
                ]}
            />
            {gpus.length > 0 && (
                <div style={{ marginTop: space[4] }}>
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[100px]">vendor</TableHead>
                                <TableHead>model</TableHead>
                                <TableHead className="w-[120px]">driver</TableHead>
                                <TableHead className="w-[120px]">VRAM</TableHead>
                                <TableHead className="w-[90px]">util</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {gpus.map((g, i) => (
                                <TableRow key={g.uuid || g.bus_id || `gpu-${i}`}>
                                    <TableCell>{g.vendor || "—"}</TableCell>
                                    <TableCell>{g.model || "—"}</TableCell>
                                    <TableCell>
                                        {g.driver ? (
                                            <Mono size={11}>
                                                {g.driver}
                                                {g.driver_version
                                                    ? ` ${g.driver_version}`
                                                    : ""}
                                            </Mono>
                                        ) : (
                                            "—"
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        {g.vram_total_bytes
                                            ? `${formatBytes(g.vram_used_bytes)} / ${formatBytes(
                                                  g.vram_total_bytes,
                                              )}`
                                            : "—"}
                                    </TableCell>
                                    <TableCell>
                                        {g.utilization_pct !== undefined &&
                                        g.utilization_pct > 0
                                            ? `${g.utilization_pct.toFixed(0)} %`
                                            : "—"}
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </div>
            )}
            {gpus.length === 0 && host.gpu_summary && (
                <div
                    style={{
                        marginTop: space[3],
                        fontSize: 12,
                        color: palette.textSecondary,
                    }}
                >
                    {host.gpu_summary}
                </div>
            )}
        </Card>
    );
}
