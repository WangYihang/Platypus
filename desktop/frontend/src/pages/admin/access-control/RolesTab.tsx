import { useState } from "react";
import { Loader2, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import Card from "../../../components/Card";
import EmptyState from "../../../components/EmptyState";
import FilterToolbar from "../../../components/FilterToolbar";
import Mono from "../../../components/Mono";
import StatusPill from "../../../components/StatusPill";
import { palette } from "../../../layout/theme";
import { humanizeError } from "../../../lib/humanizeError";
import {
    type RBACPermission,
    type RBACRole,
    type RBACRoleSummary,
    deleteRBACRole,
    getRBACRole,
    listRBACPermissions,
    listRBACRoles,
} from "../../../lib/api";
import { qk } from "../../../lib/queryKeys";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

import CreateRoleDialog from "./CreateRoleDialog";
import EditRoleDialog from "./EditRoleDialog";
import ErrorBox from "./ErrorBox";

export default function RolesTab() {
    const queryClient = useQueryClient();
    const [createOpen, setCreateOpen] = useState(false);
    const [editing, setEditing] = useState<RBACRole | null>(null);
    const [pendingDelete, setPendingDelete] = useState<RBACRoleSummary | null>(null);

    const rolesQuery = useQuery({
        queryKey: qk.adminRoles(),
        queryFn: () => listRBACRoles(),
    });
    const permissionsQuery = useQuery({
        queryKey: qk.adminPermissions(),
        queryFn: () => listRBACPermissions(),
    });
    const rows: RBACRoleSummary[] | null = rolesQuery.data ?? null;
    const permissions: RBACPermission[] = permissionsQuery.data ?? [];
    const loading = rolesQuery.isFetching || permissionsQuery.isFetching;
    const error = rolesQuery.error ?? permissionsQuery.error ?? null;

    function refresh() {
        queryClient.invalidateQueries({ queryKey: qk.adminRoles() });
        queryClient.invalidateQueries({ queryKey: qk.adminPermissions() });
    }

    async function openEdit(slug: string) {
        try {
            const r = await getRBACRole(slug);
            setEditing(r);
        } catch (e) {
            toast.error(`Couldn't open role: ${humanizeError(e)}`);
        }
    }

    async function confirmDelete() {
        if (!pendingDelete) return;
        const r = pendingDelete;
        setPendingDelete(null);
        try {
            await deleteRBACRole(r.slug);
            toast.success("Role deleted");
            refresh();
        } catch (e) {
            toast.error(`Couldn't delete: ${humanizeError(e)}`);
        }
    }

    return (
        <>
            <FilterToolbar
                count={rows ? `${rows.length} role${rows.length === 1 ? "" : "s"}` : null}
                onRefresh={() => refresh()}
                refreshLoading={loading}
                actions={
                    <Button size="sm" onClick={() => setCreateOpen(true)}>
                        <Plus className="size-3.5" />
                        New role
                    </Button>
                }
            />
            {error && <ErrorBox text={String(error)} />}
            <Card padding={0}>
                {rows === null ? (
                    <div className="flex items-center justify-center p-10">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                ) : rows.length === 0 ? (
                    <EmptyState
                        icon={<ShieldCheck className="size-7" />}
                        title="No roles defined"
                        description="Builtin roles (viewer / operator / admin) ship with the server. Create custom roles to fit your team."
                    />
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[180px]">Name</TableHead>
                                <TableHead className="w-[140px]">Slug</TableHead>
                                <TableHead>Slots</TableHead>
                                <TableHead className="w-[120px]">Type</TableHead>
                                <TableHead className="w-[80px]" />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((r) => (
                                <TableRow
                                    key={r.slug}
                                    className="cursor-pointer hover:bg-surface-hover"
                                    onClick={() => openEdit(r.slug)}
                                >
                                    <TableCell>
                                        <div style={{ fontWeight: 500 }}>{r.name}</div>
                                        {r.description && (
                                            <div
                                                style={{
                                                    color: palette.textMuted,
                                                    fontSize: 12,
                                                }}
                                            >
                                                {r.description}
                                            </div>
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        <Mono size={12}>{r.slug}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        <span className="text-text-muted text-xs">
                                            {[
                                                r.is_global ? "global" : null,
                                                r.is_project ? "project" : null,
                                            ]
                                                .filter(Boolean)
                                                .join(" · ")}
                                        </span>
                                    </TableCell>
                                    <TableCell>
                                        <StatusPill tone={r.is_builtin ? "info" : "neutral"}>
                                            {r.is_builtin ? "builtin" : "custom"}
                                        </StatusPill>
                                    </TableCell>
                                    <TableCell>
                                        {!r.is_builtin && (
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                                                aria-label={`Delete ${r.slug}`}
                                                onClick={(e) => {
                                                    e.stopPropagation();
                                                    setPendingDelete(r);
                                                }}
                                            >
                                                <Trash2 className="size-3.5" />
                                            </Button>
                                        )}
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                )}
            </Card>

            <CreateRoleDialog
                open={createOpen}
                onOpenChange={(o) => {
                    setCreateOpen(o);
                    if (!o) refresh();
                }}
                permissions={permissions}
            />
            <EditRoleDialog
                role={editing}
                onOpenChange={(o) => {
                    if (!o) {
                        setEditing(null);
                        refresh();
                    }
                }}
                permissions={permissions}
            />

            <AlertDialog
                open={pendingDelete !== null}
                onOpenChange={(o) => !o && setPendingDelete(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Delete role?</AlertDialogTitle>
                        <AlertDialogDescription>
                            <Mono>{pendingDelete?.slug}</Mono> will be removed. The
                            request fails with 409 if any user or project member is
                            still assigned this role.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={confirmDelete}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Delete
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}
