import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Copy, KeyRound, Loader2, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../components/Card";
import DataList from "../components/DataList";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import RefreshButton from "../components/RefreshButton";
import PageHeader from "../components/PageHeader";
import Toolbar from "../components/Toolbar";
import { palette, space } from "../layout/theme";
import { fromNow } from "../lib/time";
import { changePassword, getSessionUser } from "../lib/auth";
import { humanizeError } from "../lib/humanizeError";
import {
    type AccountPAT,
    type IssueAccountPATResponse,
    issueAccountPAT,
    listAccountPATs,
    listMyPermissions,
    revokeAccountPAT,
} from "../lib/api";

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

// Account is the home of *user-level, server-side* settings:
//
//   • Identity   — read-only username / role / id card.
//   • Password   — change-password form (session reset on submit).
//   • Tokens     — manage user-self PATs (`pat_*`).
//
// Distinct from /preferences (browser-local). The PageHeader subtitle
// names the scope ("Signed in as X") so the surface reads honestly.
const passwordSchema = z
    .object({
        old_password: z.string().min(1, "current password is required"),
        new_password: z.string().min(8, "Min 8 chars"),
        confirm: z.string().min(1, "required"),
    })
    .refine((v) => v.confirm === v.new_password, {
        path: ["confirm"],
        message: "passwords do not match",
    });
type PasswordFormValues = z.infer<typeof passwordSchema>;

// PAT scopes are now fetched from /api/v1/account/permissions — the
// caller's effective role permissions, post-RBAC. The hardcoded
// READ/WRITE split is gone; the dialog renders whatever the server
// reports, grouped by the resource prefix in the slug
// (hosts:* / files:* / etc.) so a long list stays readable.
function groupScopesByResource(scopes: string[]): Array<[string, string[]]> {
    const m = new Map<string, string[]>();
    for (const s of scopes) {
        const colon = s.indexOf(":");
        const resource = colon > 0 ? s.slice(0, colon) : "other";
        const list = m.get(resource) ?? [];
        list.push(s);
        m.set(resource, list);
    }
    return Array.from(m.entries()).sort((a, b) => a[0].localeCompare(b[0]));
}

export default function Account() {
    const user = getSessionUser();
    const navigate = useNavigate();

    const pwForm = useForm<PasswordFormValues>({
        resolver: zodResolver(passwordSchema),
        defaultValues: { old_password: "", new_password: "", confirm: "" },
    });

    async function handlePasswordChange(v: PasswordFormValues) {
        try {
            await changePassword(v.old_password, v.new_password);
            toast.success("Password updated — please log in again");
            navigate("/login", { replace: true });
        } catch (e) {
            toast.error(`change password: ${humanizeError(e)}`);
        }
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Account"
                subtitle={user ? `Signed in as ${user.username}` : "Account"}
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                <div style={{ maxWidth: 720 }}>
                    <Tabs defaultValue="identity">
                        <TabsList data-testid="account-tabs" className="mb-4">
                            <TabsTrigger value="identity">Identity</TabsTrigger>
                            <TabsTrigger value="password">Password</TabsTrigger>
                            <TabsTrigger value="tokens">Tokens</TabsTrigger>
                        </TabsList>

                        <TabsContent value="identity" className="space-y-4">
                            {user && (
                                <Card header="Identity" padding={5}>
                                    <DataList
                                        items={[
                                            {
                                                label: "username",
                                                value: <Mono>{user.username}</Mono>,
                                            },
                                            { label: "role", value: user.role },
                                            {
                                                label: "id",
                                                value: <Mono size={11}>{user.id}</Mono>,
                                            },
                                        ]}
                                    />
                                </Card>
                            )}
                        </TabsContent>

                        <TabsContent value="password" className="space-y-4">
                            <Card header="Change password" padding={5}>
                                <p
                                    style={{
                                        color: palette.textSecondary,
                                        fontSize: 13,
                                        lineHeight: 1.5,
                                        marginTop: 0,
                                        marginBottom: space[4],
                                    }}
                                >
                                    Updating your password signs you out everywhere — all
                                    existing sessions on this and other devices end.
                                </p>
                                <Form {...pwForm}>
                                    <form
                                        onSubmit={pwForm.handleSubmit(handlePasswordChange)}
                                        className="space-y-4"
                                    >
                                        <FormField
                                            control={pwForm.control}
                                            name="old_password"
                                            render={({ field }) => (
                                                <FormItem>
                                                    <FormLabel>Current password</FormLabel>
                                                    <FormControl>
                                                        <Input type="password" autoFocus {...field} />
                                                    </FormControl>
                                                    <FormMessage />
                                                </FormItem>
                                            )}
                                        />
                                        <FormField
                                            control={pwForm.control}
                                            name="new_password"
                                            render={({ field }) => (
                                                <FormItem>
                                                    <FormLabel>New password</FormLabel>
                                                    <FormControl>
                                                        <Input type="password" {...field} />
                                                    </FormControl>
                                                    <FormDescription>
                                                        Min 8 characters.
                                                    </FormDescription>
                                                    <FormMessage />
                                                </FormItem>
                                            )}
                                        />
                                        <FormField
                                            control={pwForm.control}
                                            name="confirm"
                                            render={({ field }) => (
                                                <FormItem>
                                                    <FormLabel>Confirm new password</FormLabel>
                                                    <FormControl>
                                                        <Input type="password" {...field} />
                                                    </FormControl>
                                                    <FormMessage />
                                                </FormItem>
                                            )}
                                        />
                                        <div className="flex justify-end">
                                            <Button
                                                type="submit"
                                                disabled={pwForm.formState.isSubmitting}
                                            >
                                                {pwForm.formState.isSubmitting && (
                                                    <Loader2 className="size-3.5 animate-spin" />
                                                )}
                                                Update password
                                            </Button>
                                        </div>
                                    </form>
                                </Form>
                            </Card>
                        </TabsContent>

                        <TabsContent value="tokens" className="space-y-4">
                            <AccountTokensTab />
                        </TabsContent>
                    </Tabs>
                </div>
            </div>
        </div>
    );
}

function AccountTokensTab() {
    const [rows, setRows] = useState<AccountPAT[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [includeRevoked, setIncludeRevoked] = useState(false);
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssueAccountPATResponse | null>(null);
    const [pendingRevoke, setPendingRevoke] = useState<AccountPAT | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listAccountPATs(includeRevoked);
            setRows(data);
            setError(null);
        } catch (e) {
            setError(humanizeError(e));
            toast.error(`Couldn't load tokens: ${humanizeError(e)}`);
        } finally {
            setLoading(false);
        }
    }, [includeRevoked]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function confirmRevoke() {
        if (!pendingRevoke) return;
        const r = pendingRevoke;
        setPendingRevoke(null);
        try {
            await revokeAccountPAT(r.token_id);
            toast.success("Token revoked");
            refresh();
        } catch (e) {
            toast.error(`Couldn't revoke: ${humanizeError(e)}`);
        }
    }

    return (
        <>
            <p
                style={{
                    color: palette.textSecondary,
                    fontSize: 13,
                    lineHeight: 1.5,
                    marginTop: 0,
                    marginBottom: space[3],
                }}
            >
                Personal access tokens authenticate API requests as your user.
                Pass them as <Mono>Authorization: Bearer pat_…</Mono>.
            </p>
            <Toolbar
                left={
                    <label
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: space[2],
                            fontSize: 13,
                            color: palette.textSecondary,
                            cursor: "pointer",
                        }}
                    >
                        <Checkbox
                            checked={includeRevoked}
                            onCheckedChange={(v) => setIncludeRevoked(Boolean(v))}
                        />
                        Show revoked
                    </label>
                }
                right={
                    <>
                        <RefreshButton loading={loading} onClick={refresh} />
                        <Button size="sm" onClick={() => setIssueOpen(true)}>
                            <Plus className="size-3.5" />
                            Issue token
                        </Button>
                    </>
                }
            />
            {error && (
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
                    {error}
                </div>
            )}
            <Card padding={0}>
                {rows === null ? (
                    <div className="flex items-center justify-center p-10">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                ) : rows.length === 0 ? (
                    <EmptyState
                        icon={<KeyRound className="size-7" />}
                        title="No personal access tokens"
                        description="Tokens you issue will appear here. The plaintext shows once at creation — copy it then."
                    />
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[180px]">Name</TableHead>
                                <TableHead>Scopes</TableHead>
                                <TableHead className="w-[140px]">Last used</TableHead>
                                <TableHead className="w-[140px]">Expires</TableHead>
                                <TableHead className="w-[60px]" />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((r) => (
                                <TableRow
                                    key={r.token_id}
                                    style={{ opacity: r.revoked ? 0.55 : 1 }}
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
                                        <span
                                            style={{
                                                fontFamily: "var(--font-mono)",
                                                fontSize: 11,
                                                color: palette.textSecondary,
                                            }}
                                        >
                                            {r.scopes.join(" ")}
                                        </span>
                                    </TableCell>
                                    <TableCell>
                                        {r.last_used_at ? (
                                            fromNow(r.last_used_at)
                                        ) : (
                                            <span className="text-text-muted">never</span>
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        {r.revoked
                                            ? "revoked"
                                            : fromNow(r.expires_at)}
                                    </TableCell>
                                    <TableCell>
                                        {!r.revoked && (
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                                                onClick={() => setPendingRevoke(r)}
                                                aria-label={`Revoke ${r.name}`}
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

            <IssueAccountPATDialog
                open={issueOpen}
                onOpenChange={(o) => {
                    setIssueOpen(o);
                    if (!o) refresh();
                }}
                onIssued={(r) => {
                    setLastIssued(r);
                    setIssueOpen(false);
                    refresh();
                }}
            />
            <IssuedAccountPATDialog
                result={lastIssued}
                onClose={() => setLastIssued(null)}
            />

            <AlertDialog
                open={pendingRevoke !== null}
                onOpenChange={(o) => !o && setPendingRevoke(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Revoke this token?</AlertDialogTitle>
                        <AlertDialogDescription>
                            Anyone using <Mono>{pendingRevoke?.name}</Mono> will start
                            getting 401s on the next request.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={confirmRevoke}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Revoke
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}

function IssueAccountPATDialog({
    open,
    onOpenChange,
    onIssued,
}: {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onIssued: (r: IssueAccountPATResponse) => void;
}) {
    const [name, setName] = useState("");
    const [description, setDescription] = useState("");
    // available is the caller's effective permission set, fetched on
    // open. selected is the subset they want to put on the new PAT.
    const [available, setAvailable] = useState<string[] | null>(null);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [ttlDays, setTtlDays] = useState<number>(90);
    const [submitting, setSubmitting] = useState(false);

    // Reset + refetch on open so each new token starts clean and
    // reflects any role changes since the page mounted.
    useEffect(() => {
        if (!open) return;
        setName("");
        setDescription("");
        setTtlDays(90);
        setSubmitting(false);
        setAvailable(null);
        setSelected(new Set());
        listMyPermissions()
            .then((perms) => {
                setAvailable(perms);
                setSelected(new Set(perms));
            })
            .catch((e) => {
                toast.error(`Couldn't load permissions: ${humanizeError(e)}`);
                setAvailable([]);
            });
    }, [open]);

    function toggleScope(s: string) {
        setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(s)) next.delete(s);
            else next.add(s);
            return next;
        });
    }

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        if (!name.trim() || selected.size === 0) return;
        setSubmitting(true);
        try {
            const r = await issueAccountPAT({
                name: name.trim(),
                description: description.trim() || undefined,
                scopes: Array.from(selected),
                ttl_seconds: ttlDays * 24 * 60 * 60,
            });
            onIssued(r);
        } catch (e) {
            toast.error(`Couldn't issue: ${humanizeError(e)}`);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[520px]">
                <DialogHeader>
                    <DialogTitle>Issue personal access token</DialogTitle>
                    <DialogDescription>
                        The plaintext appears once after creation — copy it before
                        closing the next dialog.
                    </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit} className="space-y-4">
                    <div className="space-y-1">
                        <Label htmlFor="pat-name">Name</Label>
                        <Input
                            id="pat-name"
                            placeholder="e.g. ci-runner"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            autoFocus
                        />
                    </div>
                    <div className="space-y-1">
                        <Label htmlFor="pat-desc">Description (optional)</Label>
                        <Textarea
                            id="pat-desc"
                            rows={2}
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                        />
                    </div>

                    <div className="space-y-2">
                        <Label>Scopes</Label>
                        {available === null ? (
                            <div className="flex items-center gap-2 text-text-muted text-xs">
                                <Loader2 className="size-3.5 animate-spin" />
                                Loading your permissions…
                            </div>
                        ) : available.length === 0 ? (
                            <p
                                style={{
                                    color: palette.textMuted,
                                    fontSize: 12,
                                    margin: 0,
                                }}
                            >
                                Your role doesn't grant any permissions — there's
                                nothing to scope a token to. Ask an admin to update
                                your role.
                            </p>
                        ) : (
                            groupScopesByResource(available).map(([resource, perms]) => (
                                <ScopeGroup
                                    key={resource}
                                    label={resource}
                                    options={perms}
                                    selected={selected}
                                    onToggle={toggleScope}
                                />
                            ))
                        )}
                    </div>

                    <div className="space-y-1">
                        <Label htmlFor="pat-ttl">Expires in (days)</Label>
                        <Input
                            id="pat-ttl"
                            type="number"
                            min={1}
                            max={365}
                            value={ttlDays}
                            onChange={(e) =>
                                setTtlDays(Math.max(1, Math.min(365, Number(e.target.value) || 90)))
                            }
                        />
                    </div>

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="outline"
                            onClick={() => onOpenChange(false)}
                        >
                            Cancel
                        </Button>
                        <Button
                            type="submit"
                            disabled={
                                submitting || !name.trim() || selected.size === 0
                            }
                        >
                            {submitting && (
                                <Loader2 className="size-3.5 animate-spin" />
                            )}
                            Issue token
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}

function ScopeGroup({
    label,
    options,
    selected,
    onToggle,
}: {
    label: string;
    options: readonly string[];
    selected: Set<string>;
    onToggle: (s: string) => void;
}) {
    return (
        <div>
            <div
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    marginBottom: space[1],
                    textTransform: "uppercase",
                    letterSpacing: 0.5,
                }}
            >
                {label}
            </div>
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
                    gap: space[2],
                }}
            >
                {options.map((s) => (
                    <label
                        key={s}
                        htmlFor={`scope-${s}`}
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: space[2],
                            fontSize: 13,
                            color: palette.textPrimary,
                            cursor: "pointer",
                        }}
                    >
                        <Checkbox
                            id={`scope-${s}`}
                            checked={selected.has(s)}
                            onCheckedChange={() => onToggle(s)}
                        />
                        <Mono size={12}>{s}</Mono>
                    </label>
                ))}
            </div>
        </div>
    );
}

function IssuedAccountPATDialog({
    result,
    onClose,
}: {
    result: IssueAccountPATResponse | null;
    onClose: () => void;
}) {
    async function copy() {
        if (!result) return;
        try {
            await navigator.clipboard.writeText(result.token);
            toast.success("Token copied");
        } catch {
            toast.error("Copy failed — select manually");
        }
    }

    return (
        <Dialog open={result !== null} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="sm:max-w-[560px]">
                <DialogHeader>
                    <DialogTitle>Token issued</DialogTitle>
                    <DialogDescription>
                        Copy this token now. After you close this dialog it can never
                        be retrieved again — issue a new one if you lose it.
                    </DialogDescription>
                </DialogHeader>
                {result && (
                    <div className="space-y-3">
                        <div
                            style={{
                                fontFamily: "var(--font-mono)",
                                fontSize: 12,
                                background: palette.surface,
                                border: `1px solid ${palette.border}`,
                                padding: `${space[3]}px ${space[4]}px`,
                                borderRadius: 6,
                                wordBreak: "break-all",
                            }}
                        >
                            {result.token}
                        </div>
                        <DataList
                            items={[
                                { label: "name", value: result.name },
                                {
                                    label: "scopes",
                                    value: <Mono size={12}>{result.scopes.join(" ")}</Mono>,
                                },
                                { label: "expires", value: fromNow(result.expires_at) },
                            ]}
                        />
                    </div>
                )}
                <DialogFooter>
                    <Button variant="outline" onClick={copy}>
                        <Copy className="size-3.5" />
                        Copy
                    </Button>
                    <Button onClick={onClose}>Done</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
