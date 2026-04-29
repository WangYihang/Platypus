import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import RemoteAddr from "../../components/RemoteAddr";
import StatusPill from "../../components/StatusPill";
import { SessionRow } from "../../lib/api";
import { fromNow } from "../../lib/time";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

interface Props {
    sessions: SessionRow[];
}

// SessionsTab renders the host's connection log: every session that
// has dialled this host, including closed ones, ordered by the
// backend (newest first). Used inside HostView's "Sessions" tab.
//
// Root sessions get a danger pill on the User column so an operator
// scanning the table notices the privilege level without parsing
// the entire row. Closed sessions render a neutral "closed Xm ago"
// pill so the most recent live one is still easy to spot.
export default function SessionsTab({ sessions }: Props) {
    if (sessions.length === 0) {
        return (
            <Card padding={0}>
                <EmptyState
                    title="No sessions"
                    description="No connections recorded for this host yet."
                />
            </Card>
        );
    }
    return (
        <Card padding={0}>
            <Table>
                <TableHeader>
                    <TableRow>
                        <TableHead className="w-[180px]">Session</TableHead>
                        <TableHead>Ingress</TableHead>
                        <TableHead>User</TableHead>
                        <TableHead>Remote</TableHead>
                        <TableHead className="w-[120px]">Agent</TableHead>
                        <TableHead className="w-[140px]">Connected</TableHead>
                        <TableHead className="w-[180px]">Status</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {sessions.map((r) => (
                        <TableRow key={r.id}>
                            <TableCell>
                                <Mono>{`${r.id.slice(0, 16)}…`}</Mono>
                            </TableCell>
                            <TableCell>
                                {r.ingress_addr ? <Mono>{r.ingress_addr}</Mono> : "—"}
                            </TableCell>
                            <TableCell>
                                {r.user ? (
                                    r.user === "root" ? (
                                        <StatusPill tone="danger">root</StatusPill>
                                    ) : (
                                        <Mono>{r.user}</Mono>
                                    )
                                ) : (
                                    "—"
                                )}
                            </TableCell>
                            <TableCell>
                                {r.remote_addr ? (
                                    <RemoteAddr addr={r.remote_addr} info={r.remote_info} />
                                ) : (
                                    "—"
                                )}
                            </TableCell>
                            <TableCell data-testid="session-version-cell">
                                {r.version ? <Mono size={11}>{r.version}</Mono> : "—"}
                            </TableCell>
                            <TableCell className="text-text-secondary">
                                {fromNow(r.connected_at)}
                            </TableCell>
                            <TableCell>
                                {r.disconnected_at ? (
                                    <StatusPill tone="neutral">
                                        {`closed ${fromNow(r.disconnected_at)}`}
                                    </StatusPill>
                                ) : (
                                    <StatusPill tone="success">live</StatusPill>
                                )}
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>
        </Card>
    );
}
