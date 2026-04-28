import { useEffect, useMemo, useState } from "react";
import { Loader2, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import PageShell from "../../components/PageShell";
import RefreshButton from "../../components/RefreshButton";
import StatusPill from "../../components/StatusPill";
import Toolbar from "../../components/Toolbar";
import { palette, space } from "../../layout/theme";
import { humanizeError } from "../../lib/humanizeError";
import {
    type CreateRBACRoleRequest,
    type RBACPermission,
    type RBACRole,
    type RBACRoleSummary,
    createRBACRole,
    deleteRBACRole,
    getRBACRole,
    listRBACPermissions,
    listRBACRoles,
    updateRBACRole,
} from "../../lib/api";
import { qk } from "../../lib/queryKeys";

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
import { Checkbox } from "@/components/ui/checkbox";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";

// AdminAccessControl is the /admin/access-control page — admin-only
// surface that drives the RBAC tables introduced in storage migration
// 18. Two tabs:
//
//   • Roles       — list / create / edit / delete. Builtins (viewer
//                   / operator / admin) cannot be deleted; their slot
//                   affinity (is_global / is_project) is locked, but
//                   the permission set is editable.
//   • Permissions — read-only catalogue, sorted by resource. Helps
//                   admins understand the available capabilities
//                   before designing custom roles.
export default function AdminAccessControl() {
    return (
        <PageShell
            title="Access control"
            subtitle="Roles and permissions · admin only"
            bodyPadding={8}
        >
            <div style={{ maxWidth: 960 }}>
                <Tabs defaultValue="roles">
                    <TabsList data-testid="access-control-tabs" className="mb-4">
                        <TabsTrigger value="roles">Roles</TabsTrigger>
                        <TabsTrigger value="permissions">Permissions</TabsTrigger>
                    </TabsList>
                    <TabsContent value="roles" className="space-y-4">
                        <RolesTab />
                    </TabsContent>
                    <TabsContent value="permissions" className="space-y-4">
                        <PermissionsTab />
                    </TabsContent>
                </Tabs>
            </div>
        </PageShell>
    );
}

function RolesTab() {
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
            <Toolbar
                right={
                    <>
                        <RefreshButton loading={loading} onClick={() => refresh()} />
                        <Button size="sm" onClick={() => setCreateOpen(true)}>
                            <Plus className="size-3.5" />
                            New role
                        </Button>
                    </>
                }
            />
            {error && (
                <ErrorBox text={String(error)} />
            )}
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

function PermissionsTab() {
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
    if (error) {
        return <ErrorBox text={String(error)} />;
    }
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

function CreateRoleDialog({
    open,
    onOpenChange,
    permissions,
}: {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    permissions: RBACPermission[];
}) {
    const [slug, setSlug] = useState("");
    const [name, setName] = useState("");
    const [description, setDescription] = useState("");
    const [isGlobal, setIsGlobal] = useState(true);
    const [isProject, setIsProject] = useState(true);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (open) {
            setSlug("");
            setName("");
            setDescription("");
            setIsGlobal(true);
            setIsProject(true);
            setSelected(new Set());
            setSubmitting(false);
        }
    }, [open]);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        if (!slug.trim() || !name.trim()) return;
        if (!isGlobal && !isProject) return;
        setSubmitting(true);
        const req: CreateRBACRoleRequest = {
            slug: slug.trim(),
            name: name.trim(),
            description: description.trim() || undefined,
            is_global: isGlobal,
            is_project: isProject,
            permissions: Array.from(selected),
        };
        try {
            await createRBACRole(req);
            toast.success("Role created");
            onOpenChange(false);
        } catch (e) {
            toast.error(`Couldn't create: ${humanizeError(e)}`);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[640px] max-h-[90vh] overflow-auto">
                <DialogHeader>
                    <DialogTitle>New role</DialogTitle>
                    <DialogDescription>
                        Slugs are stable identifiers stored in users.role and
                        project_members.role. Use lowercase letters and hyphens.
                    </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit} className="space-y-4">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1">
                            <Label htmlFor="role-slug">Slug</Label>
                            <Input
                                id="role-slug"
                                value={slug}
                                onChange={(e) => setSlug(e.target.value)}
                                placeholder="oncall"
                                autoFocus
                            />
                        </div>
                        <div className="space-y-1">
                            <Label htmlFor="role-name">Name</Label>
                            <Input
                                id="role-name"
                                value={name}
                                onChange={(e) => setName(e.target.value)}
                                placeholder="On-call Operator"
                            />
                        </div>
                    </div>
                    <div className="space-y-1">
                        <Label htmlFor="role-desc">Description (optional)</Label>
                        <Textarea
                            id="role-desc"
                            rows={2}
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                        />
                    </div>
                    <div className="flex gap-4">
                        <label className="inline-flex items-center gap-2 text-sm">
                            <Checkbox
                                checked={isGlobal}
                                onCheckedChange={(v) => setIsGlobal(Boolean(v))}
                            />
                            Assignable globally
                        </label>
                        <label className="inline-flex items-center gap-2 text-sm">
                            <Checkbox
                                checked={isProject}
                                onCheckedChange={(v) => setIsProject(Boolean(v))}
                            />
                            Assignable per-project
                        </label>
                    </div>
                    <PermissionMatrix
                        permissions={permissions}
                        selected={selected}
                        onToggle={(slug) => {
                            const next = new Set(selected);
                            if (next.has(slug)) next.delete(slug);
                            else next.add(slug);
                            setSelected(next);
                        }}
                    />
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            Cancel
                        </Button>
                        <Button
                            type="submit"
                            disabled={
                                submitting ||
                                !slug.trim() ||
                                !name.trim() ||
                                (!isGlobal && !isProject)
                            }
                        >
                            {submitting && <Loader2 className="size-3.5 animate-spin" />}
                            Create role
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}

function EditRoleDialog({
    role,
    onOpenChange,
    permissions,
}: {
    role: RBACRole | null;
    onOpenChange: (o: boolean) => void;
    permissions: RBACPermission[];
}) {
    const [name, setName] = useState("");
    const [description, setDescription] = useState("");
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (role) {
            setName(role.name);
            setDescription(role.description ?? "");
            setSelected(new Set(role.permissions));
            setSubmitting(false);
        }
    }, [role]);

    if (!role) return null;

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        if (!role) return;
        setSubmitting(true);
        try {
            await updateRBACRole(role.slug, {
                name: role.is_builtin ? undefined : name,
                description,
                permissions: Array.from(selected),
            });
            toast.success("Role updated");
            onOpenChange(false);
        } catch (e) {
            toast.error(`Couldn't update: ${humanizeError(e)}`);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[640px] max-h-[90vh] overflow-auto">
                <DialogHeader>
                    <DialogTitle>
                        {role.is_builtin ? "Edit builtin role" : "Edit role"}: {role.name}
                    </DialogTitle>
                    <DialogDescription>
                        {role.is_builtin
                            ? "Builtin role names and slot affinity are locked. The permission set can still be tuned."
                            : "Name and permission set are editable."}
                    </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit} className="space-y-4">
                    <div className="space-y-1">
                        <Label htmlFor="edit-role-name">Name</Label>
                        <Input
                            id="edit-role-name"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            disabled={role.is_builtin}
                        />
                    </div>
                    <div className="space-y-1">
                        <Label htmlFor="edit-role-desc">Description</Label>
                        <Textarea
                            id="edit-role-desc"
                            rows={2}
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                        />
                    </div>
                    <PermissionMatrix
                        permissions={permissions}
                        selected={selected}
                        onToggle={(slug) => {
                            const next = new Set(selected);
                            if (next.has(slug)) next.delete(slug);
                            else next.add(slug);
                            setSelected(next);
                        }}
                    />
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            Cancel
                        </Button>
                        <Button type="submit" disabled={submitting}>
                            {submitting && <Loader2 className="size-3.5 animate-spin" />}
                            Save
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}

function PermissionMatrix({
    permissions,
    selected,
    onToggle,
}: {
    permissions: RBACPermission[];
    selected: Set<string>;
    onToggle: (slug: string) => void;
}) {
    const grouped = useMemo(() => {
        const m = new Map<string, RBACPermission[]>();
        for (const p of permissions) {
            const list = m.get(p.resource) ?? [];
            list.push(p);
            m.set(p.resource, list);
        }
        return Array.from(m.entries()).sort((a, b) => a[0].localeCompare(b[0]));
    }, [permissions]);

    return (
        <div className="space-y-3">
            <Label>Permissions</Label>
            {grouped.map(([resource, perms]) => (
                <div key={resource}>
                    <div
                        style={{
                            fontSize: 12,
                            color: palette.textMuted,
                            marginBottom: space[1],
                            textTransform: "uppercase",
                            letterSpacing: 0.5,
                        }}
                    >
                        {resource}
                    </div>
                    <div
                        style={{
                            display: "grid",
                            gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
                            gap: space[2],
                        }}
                    >
                        {perms.map((p) => (
                            <label
                                key={p.slug}
                                htmlFor={`perm-${p.slug}`}
                                style={{
                                    display: "flex",
                                    alignItems: "flex-start",
                                    gap: space[2],
                                    fontSize: 13,
                                    cursor: "pointer",
                                }}
                            >
                                <Checkbox
                                    id={`perm-${p.slug}`}
                                    checked={selected.has(p.slug)}
                                    onCheckedChange={() => onToggle(p.slug)}
                                />
                                <div>
                                    <Mono size={12}>{p.slug}</Mono>
                                    <div
                                        style={{
                                            color: palette.textMuted,
                                            fontSize: 11,
                                            marginTop: 2,
                                        }}
                                    >
                                        {p.description}
                                    </div>
                                </div>
                            </label>
                        ))}
                    </div>
                </div>
            ))}
        </div>
    );
}

function ErrorBox({ text }: { text: string }) {
    return (
        <div
            style={{
                marginBottom: space[3],
                padding: `${space[3]}px ${space[4]}px`,
                border: `1px solid ${palette.danger}`,
                borderRadius: 6,
                color: palette.danger,
                fontSize: 13,
            }}
        >
            {text}
        </div>
    );
}
