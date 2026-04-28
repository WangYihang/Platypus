import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { humanizeError } from "../lib/humanizeError";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import PageHeader from "../components/PageHeader";
import RefreshButton from "../components/RefreshButton";
import RoleHelpIcon from "../components/RoleHelpIcon";
import StatusPill from "../components/StatusPill";
import { palette, space } from "../layout/theme";
import {
    Project,
    ProjectMember,
    UserRow,
    addProjectMember,
    listProjectMembers,
    listUsers,
    removeProjectMember,
} from "../lib/api";
import { getSessionUser } from "../lib/auth";
import { memberRemovalWarning } from "./members/warnings";

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
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import {
    Form,
    FormControl,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

const ROLES = ["admin", "operator", "viewer"] as const;
type Role = (typeof ROLES)[number];

const ROLE_TONE: Record<Role, "danger" | "info" | "neutral"> = {
    admin: "danger",
    operator: "info",
    viewer: "neutral",
};

const addMemberSchema = z.object({
    user_id: z.string().min(1, "pick a user"),
    role: z.enum(ROLES),
});
type AddMemberValues = z.infer<typeof addMemberSchema>;

interface Props {
    project: Project;
}

// ProjectMembers is the per-project ACL editor. Global admins can add
// anyone; project-admins can change roles and remove members but not
// add new ones — the backend's /users list is admin-only.
export default function ProjectMembers({ project }: Props) {
    const [members, setMembers] = useState<ProjectMember[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [addOpen, setAddOpen] = useState(false);
    const [candidates, setCandidates] = useState<UserRow[] | null>(null);
    const [pendingRemove, setPendingRemove] = useState<ProjectMember | null>(null);

    const me = getSessionUser();
    const canAdd = me?.role === "admin";
    // Whether the next successful submit closes the dialog.
    // "Add" → true (close); "Add another" → false (keep open and
    // reset form). Tracked as state rather than a closure variable
    // because the value has to live across the async submit.
    const [closeAfterAdd, setCloseAfterAdd] = useState(true);

    const addForm = useForm<AddMemberValues>({
        resolver: zodResolver(addMemberSchema),
        defaultValues: { user_id: "", role: "operator" },
    });

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setMembers(await listProjectMembers(project.id));
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [project.id]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function openAddDialog() {
        setAddOpen(true);
        if (!candidates) {
            try {
                setCandidates(await listUsers());
            } catch (e) {
                toast.error(`load users: ${humanizeError(e)}`);
            }
        }
    }

    async function handleAdd(v: AddMemberValues) {
        try {
            await addProjectMember(project.id, v.user_id, v.role);
            toast.success("Member added");
            // Reset the form either way; only close when the user
            // hit the "Add" button (closeAfterAdd=true). "Add
            // another" leaves the dialog up so admins can chain
            // adds without re-opening.
            addForm.reset({ user_id: "", role: "operator" });
            if (closeAfterAdd) setAddOpen(false);
            refresh();
        } catch (e) {
            toast.error(`add: ${humanizeError(e)}`);
        }
    }

    async function handleRoleChange(m: ProjectMember, role: Role) {
        try {
            await addProjectMember(project.id, m.user_id, role);
            toast.success(`${m.username} → ${role}`);
            refresh();
        } catch (e) {
            toast.error(`role: ${humanizeError(e)}`);
        }
    }

    async function confirmRemove() {
        if (!pendingRemove) return;
        const m = pendingRemove;
        setPendingRemove(null);
        try {
            await removeProjectMember(project.id, m.user_id);
            toast.success(`${m.username} removed`);
            refresh();
        } catch (e) {
            toast.error(`remove: ${humanizeError(e)}`);
        }
    }

    const existingIds = new Set(members?.map((m) => m.user_id));
    const availableCandidates = (candidates ?? []).filter((u) => !existingIds.has(u.id));

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title={`${project.name} · members`}
                subtitle={`${members?.length ?? 0} member(s)`}
                actions={
                    <>
                        <RefreshButton loading={loading} onClick={refresh} />
                        {canAdd && (
                            <Button size="sm" onClick={openAddDialog}>
                                <Plus className="size-3.5" />
                                Add member
                            </Button>
                        )}
                    </>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <div
                        style={{
                            marginBottom: space[4],
                            padding: `${space[3]}px ${space[4]}px`,
                            border: `1px solid ${palette.danger}`,
                            borderRadius: 6,
                            color: palette.danger,
                            fontSize: 13,
                        }}
                    >
                        {error}
                    </div>
                )}
                {!canAdd && (
                    <p
                        style={{
                            margin: `0 0 ${space[4]}px`,
                            color: palette.textSecondary,
                            fontSize: 13,
                            lineHeight: 1.5,
                        }}
                    >
                        Adding new members requires global admin. Ask one to add the user, then
                        you can adjust their project role from this table.
                    </p>
                )}
                {members && members.length === 0 ? (
                    <EmptyState
                        title="No members"
                        description={
                            canAdd
                                ? "Members give Operator or Viewer access to this project. Global admins (you included) can already see every project — they don't need to be members."
                                : "No members on this project yet. Global admins still have access; ask one to add a user if other operators or viewers need it."
                        }
                    />
                ) : !members ? (
                    <div className="flex items-center justify-center p-20">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                ) : (
                    <Card padding={0}>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>User</TableHead>
                                    <TableHead className="w-[180px]">
                                        Role
                                        <RoleHelpIcon />
                                    </TableHead>
                                    <TableHead className="w-[140px] text-right" />
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {members.map((m) => (
                                    <TableRow key={m.user_id}>
                                        <TableCell className="font-medium">{m.username}</TableCell>
                                        <TableCell>
                                            <Select
                                                value={m.role}
                                                onValueChange={(v) =>
                                                    handleRoleChange(m, v as Role)
                                                }
                                            >
                                                <SelectTrigger size="sm" className="min-w-[130px]">
                                                    <SelectValue />
                                                </SelectTrigger>
                                                <SelectContent>
                                                    {ROLES.map((r) => (
                                                        <SelectItem key={r} value={r}>
                                                            <StatusPill tone={ROLE_TONE[r]}>
                                                                {r}
                                                            </StatusPill>
                                                        </SelectItem>
                                                    ))}
                                                </SelectContent>
                                            </Select>
                                        </TableCell>
                                        <TableCell>
                                            <div className="flex justify-end">
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    className="text-destructive hover:text-destructive"
                                                    onClick={() => setPendingRemove(m)}
                                                >
                                                    <Trash2 className="size-3.5" />
                                                    Remove
                                                </Button>
                                            </div>
                                        </TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                    </Card>
                )}
            </div>

            {/* Add member */}
            <Dialog
                open={addOpen}
                onOpenChange={(o) => {
                    setAddOpen(o);
                    if (!o) addForm.reset({ user_id: "", role: "operator" });
                }}
            >
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>Add project member</DialogTitle>
                        <DialogDescription>
                            Grant a global user access to this project.
                        </DialogDescription>
                    </DialogHeader>
                    <Form {...addForm}>
                        <form onSubmit={addForm.handleSubmit(handleAdd)} className="space-y-4">
                            <FormField
                                control={addForm.control}
                                name="user_id"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>User</FormLabel>
                                        <Select
                                            value={field.value}
                                            onValueChange={field.onChange}
                                        >
                                            <FormControl>
                                                <SelectTrigger>
                                                    <SelectValue placeholder="Select a user" />
                                                </SelectTrigger>
                                            </FormControl>
                                            <SelectContent>
                                                {availableCandidates.length === 0 && (
                                                    <div className="px-2 py-1.5 text-xs text-text-muted">
                                                        No eligible users
                                                    </div>
                                                )}
                                                {availableCandidates.map((u) => (
                                                    <SelectItem key={u.id} value={u.id}>
                                                        <span className="text-text-primary">
                                                            {u.username}
                                                        </span>
                                                        <span className="ml-1 text-[11px] text-text-secondary">
                                                            ({u.role})
                                                        </span>
                                                    </SelectItem>
                                                ))}
                                            </SelectContent>
                                        </Select>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={addForm.control}
                                name="role"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Project role</FormLabel>
                                        <Select
                                            value={field.value}
                                            onValueChange={field.onChange}
                                        >
                                            <FormControl>
                                                <SelectTrigger>
                                                    <SelectValue />
                                                </SelectTrigger>
                                            </FormControl>
                                            <SelectContent>
                                                {ROLES.map((r) => (
                                                    <SelectItem key={r} value={r}>
                                                        <StatusPill tone={ROLE_TONE[r]}>
                                                            {r}
                                                        </StatusPill>
                                                    </SelectItem>
                                                ))}
                                            </SelectContent>
                                        </Select>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <DialogFooter>
                                <Button
                                    type="button"
                                    variant="outline"
                                    onClick={() => setAddOpen(false)}
                                >
                                    Cancel
                                </Button>
                                <Button
                                    type="submit"
                                    variant="outline"
                                    disabled={addForm.formState.isSubmitting}
                                    onClick={() => setCloseAfterAdd(false)}
                                    title="Add this member and keep the dialog open"
                                >
                                    {addForm.formState.isSubmitting && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Add another
                                </Button>
                                <Button
                                    type="submit"
                                    disabled={addForm.formState.isSubmitting}
                                    onClick={() => setCloseAfterAdd(true)}
                                >
                                    {addForm.formState.isSubmitting && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Add
                                </Button>
                            </DialogFooter>
                        </form>
                    </Form>
                </DialogContent>
            </Dialog>

            {/* Remove member */}
            <AlertDialog
                open={pendingRemove !== null}
                onOpenChange={(o) => !o && setPendingRemove(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            Remove {pendingRemove?.username} from {project.slug}?
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                            They lose access to this project. Other projects and their global
                            account are untouched.
                        </AlertDialogDescription>
                        {pendingRemove && (() => {
                            const warning = memberRemovalWarning({
                                memberCount: members?.length ?? 0,
                                isProjectAdmin: pendingRemove.role === "admin",
                            });
                            return warning ? (
                                <div
                                    data-testid="member-remove-warning"
                                    style={{
                                        marginTop: space[2],
                                        padding: `${space[2]}px ${space[3]}px`,
                                        border: `1px solid ${palette.warning}`,
                                        borderRadius: 6,
                                        color: palette.warning,
                                        fontSize: 12,
                                        lineHeight: 1.5,
                                    }}
                                >
                                    {warning}
                                </div>
                            ) : null;
                        })()}
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={confirmRemove}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Remove
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </div>
    );
}
