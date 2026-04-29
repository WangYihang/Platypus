import { useMemo } from "react";
import { Loader2 } from "lucide-react";
import { useQuery } from "@tanstack/react-query";

import Card from "../../../components/Card";
import Mono from "../../../components/Mono";
import { palette } from "../../../layout/theme";
import { type RBACPermission, listRBACPermissions } from "../../../lib/api";
import { humanizeError } from "../../../lib/humanizeError";
import { qk } from "../../../lib/queryKeys";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";

import ErrorBox from "./ErrorBox";

export default function PermissionsTab() {
    const { data: rows = null, error } = useQuery({
        queryKey: qk.adminPermissions(),
        queryFn: () => listRBACPermissions(),
    });

    const grouped = useMemo(() => {
        if (!rows) return null;
        const m = new Map<string, RBACPermission[]>();
        for (const p of rows) {
            const list = m.get(p.resource) ?? [];
            list.push(p);
            m.set(p.resource, list);
        }
        return Array.from(m.entries()).sort((a, b) => a[0].localeCompare(b[0]));
    }, [rows]);

    if (rows === null) {
        return (
            <Card padding={5}>
                <div className="flex items-center justify-center p-6">
                    <Loader2 className="size-5 animate-spin text-text-muted" />
                </div>
            </Card>
        );
    }
    if (error) return <ErrorBox text={humanizeError(error)} />;

    return (
        <div className="space-y-4">
            {grouped?.map(([resource, perms]) => (
                <Card key={resource} header={resource} padding={0}>
                    <Table>
                        <TableBody>
                            {perms.map((p) => (
                                <TableRow key={p.slug}>
                                    <TableCell className="w-[220px]">
                                        <Mono size={12}>{p.slug}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        <span style={{ color: palette.textSecondary, fontSize: 13 }}>
                                            {p.description}
                                        </span>
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </Card>
            ))}
        </div>
    );
}
