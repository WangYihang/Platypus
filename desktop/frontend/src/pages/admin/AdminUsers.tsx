import { useState } from "react";
import { Loader2, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { humanizeError } from "../../lib/humanizeError";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import PageShell from "../../components/PageShell";
import RefreshButton from "../../components/RefreshButton";
import RoleHelpIcon from "../../components/RoleHelpIcon";
import StatusPills from "../../components/StatusPills";
import StatusPill from "../../components/StatusPill";
import { palette, space } from "../../layout/theme";
import {
    type RBACRoleSummary,
    UserRow,
    createUser,
    deleteUser,
    listRBACRoles,
    listUsers,
    updateUser,
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
    FormDescription,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
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

// Fallback role list used before the dynamic /admin/roles fetch
// resolves, and as the dropdown contents on a network failure. Always
// includes the three builtins so the page never renders an empty
// Select. A custom role added via /admin/access-control surfaces in
// the dropdown on the next page mount.
const DEFAULT_ROLES = ["admin", "operator", "viewer"] as const;

const BUILTIN_ROLE_TONE: Record<string, "danger" | "info" | "neutral"> = {
    admin: "danger",
    operator: "info",
    viewer: "neutral",
};

const createUserSchema = z.object({
    username: z.string().min(1, "username is required"),
    password: z.string().min(8, "Min 8 chars"),
    role: z.string().min(1, "role is required"),
});
type CreateUserValues = z.infer<typeof createUserSchema>;

const resetPasswordSchema = z.object({
    password: z.string().min(8, "Min 8 chars"),
});
type ResetPasswordValues = z.infer<typeof resetPasswordSchema>;

// AdminUsers is the /users admin surface: list / create / change-role /
// change-password / delete. Single-table view so admins can scan the
// whole roster without scrolling through cards.
export default function AdminUsers() {
    const queryClient = useQueryClient();
    const [createOpen, setCreateOpen] = useState(false);
    const [pwOpen, setPwOpen] = useState<string | null>(null);
    const [pendingDelete, setPendingDelete] = useState<UserRow | null>(null);

    const createForm = useForm<CreateUserValues>({
        resolver: zodResolver(createUserSchema),
        defaultValues: { username: "", password: "", role: "operator" },
    });
    const pwForm = useForm<ResetPasswordValues>({
        resolver: zodResolver(resetPasswordSchema),
        defaultValues: { password: "" },
    });

    const {
        data: users,
        error,
        isFetching: loading,
        refetch,
    } = useQuery({
        queryKey: qk.adminUsers(),
        queryFn: () => listUsers(),
    });

    // Dynamic role catalogue. Falls back to the three builtins until
    // /admin/roles?is_global=true resolves so the dropdown is always
    // populated. A failure stays silent — admins shouldn't see noise on
    // a transient hiccup; the user list itself surfaces the same
    // errors.
    const { data: roleOptionsData } = useQuery({
        queryKey: qk.adminRoles(),
        queryFn: () => listRBACRoles({ isGlobal: true }),
    });
    const roleOptions: RBACRoleSummary[] = roleOptionsData ?? [];

    function invalidateUsers() {
        return queryClient.invalidateQueries({ queryKey: qk.adminUsers() });
    }

    const roleSlugs =
        roleOptions.length > 0
            ? roleOptions.map((r) => r.slug)
            : (DEFAULT_ROLES as readonly string[]);

    async function handleCreate(v: CreateUserValues) {
        try {
            await createUser(v.username, v.password, v.role);
            toast.success(`Created ${v.username}`);
            setCreateOpen(false);
            createForm.reset({ username: "", password: "", role: "operator" });
            invalidateUsers();
        } catch (e) {
            toast.error(`create: ${humanizeError(e)}`);
        }
    }

    async function confirmDelete() {
        if (!pendingDelete) return;
        const u = pendingDelete;
        setPendingDelete(null);
        try {
            await deleteUser(u.id);
            toast.success(`Deleted ${u.username}`);
            invalidateUsers();
        } catch (e) {
            toast.error(`delete: ${humanizeError(e)}`);
        }
    }

    async function handleRoleChange(u: UserRow, role: string) {
        try {
            await updateUser(u.id, { role });
            toast.success(`Updated ${u.username} role`);
            invalidateUsers();
        } catch (e) {
            toast.error(`role: ${humanizeError(e)}`);
        }
    }

    async function handlePasswordReset(v: ResetPasswordValues) {
        if (!pwOpen) return;
        try {
            await updateUser(pwOpen, { password: v.password });
            toast.success("Password updated; existing sessions revoked");
            setPwOpen(null);
            pwForm.reset({ password: "" });
        } catch (e) {
            toast.error(`reset: ${humanizeError(e)}`);
        }
    }

    const roleCounts = (users ?? []).reduce(
        (acc, u) => {
            acc[u.role] = (acc[u.role] ?? 0) + 1;
            return acc;
        },
        {} as Record<string, number>,
    );

    return (
        <>
        <PageShell
            title="Users"
            subtitle="Manage who can log in and what they can do"
            pills={
                <StatusPills
                    pills={[
                        { tone: "success", count: users?.length ?? 0, label: "total" },
                        { tone: "danger", count: roleCounts.admin ?? 0, label: "admin" },
                        { tone: "info", count: roleCounts.operator ?? 0, label: "operators" },
                    ]}
                />
            }
            actions={
                <>
                    <RefreshButton loading={loading} onClick={() => void refetch()} />
                    <Button size="sm" onClick={() => setCreateOpen(true)}>
                        <Plus className="size-3.5" />
                        New user
                    </Button>
                </>
            }
            bodyPadding={8}
        >
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
                        {humanizeError(error)}
                    </div>
                )}
                {users && users.length === 0 ? (
                    <EmptyState
                        title="No users"
                        description="Create the first user via New user."
                        action={
                            <Button onClick={() => setCreateOpen(true)}>
                                <Plus className="size-3.5" />
                                New user
                            </Button>
                        }
                    />
                ) : !users ? (
                    <div className="flex items-center justify-center p-20">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                ) : (
                    <Card padding={0}>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Username</TableHead>
                                    <TableHead className="w-[180px]">
                                        Role
                                        <RoleHelpIcon />
                                    </TableHead>
                                    <TableHead className="w-[260px] text-right" />
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {users.map((u) => (
                                    <TableRow key={u.id}>
                                        <TableCell className="font-medium">{u.username}</TableCell>
                                        <TableCell>
                                            <Select
                                                value={u.role}
                                                onValueChange={(v) =>
                                                    handleRoleChange(u, v)
                                                }
                                            >
                                                <SelectTrigger size="sm" className="min-w-[130px]">
                                                    <SelectValue />
                                                </SelectTrigger>
                                                <SelectContent>
                                                    {roleSlugs.map((r) => (
                                                        <SelectItem key={r} value={r}>
                                                            <StatusPill
                                                                tone={
                                                                    BUILTIN_ROLE_TONE[r] ??
                                                                    "neutral"
                                                                }
                                                            >
                                                                {r}
                                                            </StatusPill>
                                                        </SelectItem>
                                                    ))}
                                                </SelectContent>
                                            </Select>
                                        </TableCell>
                                        <TableCell>
                                            <div className="flex justify-end gap-1">
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    onClick={() => setPwOpen(u.id)}
                                                >
                                                    Reset password
                                                </Button>
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    className="text-destructive hover:text-destructive"
                                                    onClick={() => setPendingDelete(u)}
                                                >
                                                    <Trash2 className="size-3.5" />
                                                    Delete
                                                </Button>
                                            </div>
                                        </TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                    </Card>
                )}
        </PageShell>

            {/* Create user */}
            <Dialog open={createOpen} onOpenChange={setCreateOpen}>
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>New user</DialogTitle>
                        <DialogDescription>
                            The user can change their own password after logging in.
                        </DialogDescription>
                    </DialogHeader>
                    <Form {...createForm}>
                        <form
                            onSubmit={createForm.handleSubmit(handleCreate)}
                            className="space-y-4"
                        >
                            <FormField
                                control={createForm.control}
                                name="username"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Username</FormLabel>
                                        <FormControl>
                                            <Input autoFocus {...field} />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={createForm.control}
                                name="password"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Initial password</FormLabel>
                                        <FormControl>
                                            <Input type="password" {...field} />
                                        </FormControl>
                                        <FormDescription>Min 8 characters.</FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={createForm.control}
                                name="role"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Role</FormLabel>
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
                                                {roleSlugs.map((r) => (
                                                    <SelectItem key={r} value={r}>
                                                        <StatusPill
                                                            tone={
                                                                BUILTIN_ROLE_TONE[r] ??
                                                                "neutral"
                                                            }
                                                        >
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
                                    onClick={() => setCreateOpen(false)}
                                >
                                    Cancel
                                </Button>
                                <Button type="submit" disabled={createForm.formState.isSubmitting}>
                                    {createForm.formState.isSubmitting && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Create
                                </Button>
                            </DialogFooter>
                        </form>
                    </Form>
                </DialogContent>
            </Dialog>

            {/* Reset password */}
            <Dialog open={pwOpen !== null} onOpenChange={(o) => !o && setPwOpen(null)}>
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>Reset password</DialogTitle>
                        <DialogDescription>
                            All of the user's active sessions are invalidated.
                        </DialogDescription>
                    </DialogHeader>
                    <Form {...pwForm}>
                        <form
                            onSubmit={pwForm.handleSubmit(handlePasswordReset)}
                            className="space-y-4"
                        >
                            <FormField
                                control={pwForm.control}
                                name="password"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>New password</FormLabel>
                                        <FormControl>
                                            <Input type="password" autoFocus {...field} />
                                        </FormControl>
                                        <FormDescription>Min 8 characters.</FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <DialogFooter>
                                <Button
                                    type="button"
                                    variant="outline"
                                    onClick={() => setPwOpen(null)}
                                >
                                    Cancel
                                </Button>
                                <Button type="submit" disabled={pwForm.formState.isSubmitting}>
                                    {pwForm.formState.isSubmitting && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Reset
                                </Button>
                            </DialogFooter>
                        </form>
                    </Form>
                </DialogContent>
            </Dialog>

            {/* Delete confirmation */}
            <AlertDialog
                open={pendingDelete !== null}
                onOpenChange={(o) => !o && setPendingDelete(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Delete user {pendingDelete?.username}?</AlertDialogTitle>
                        <AlertDialogDescription>
                            Their refresh tokens are revoked and they can no longer log in.
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
