import { useCallback, useEffect, useState } from "react";
import { Copy, Loader2, Plus, RotateCw, Trash2, Zap } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    InstallArtifactListItem,
    IssueInstallResponse,
    IssuePATResponse,
    PATTokenListItem,
    getServerInfo,
    issueInstallArtifact,
    issuePAT,
    listInstallArtifacts,
    listPATTokens,
    revokeInstallArtifact,
    revokePAT,
} from "../lib/api";
import { fromNow } from "../lib/time";

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
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

// Schema for Issue Install. All optional fields stay optional; the
// backend fills in defaults. Numeric fields coerce empty strings to
// undefined so RHF's optional-number round-trip works.
const installSchema = z.object({
    server_endpoint: z.string().min(1, "required"),
    target_os: z.string().optional(),
    target_arch: z.string().optional(),
    ttl_seconds: z.number().int().positive().optional(),
});
type InstallFormValues = z.infer<typeof installSchema>;

const patSchema = z.object({
    description: z.string().optional(),
    ttl_seconds: z.number().int().positive().optional(),
    max_uses: z.number().int().positive().optional(),
    binding_machine_id: z.string().optional(),
});
type PATFormValues = z.infer<typeof patSchema>;

// EnrollmentPage bundles three related admin surfaces onto one page
// because they share the same mental model — "hand an agent something
// short-lived so it can join the mesh":
//
//  1. Install commands: one-shot `curl ... | sh` bootstraps that
//     atomically mint a PAT the first time they're fetched.
//  2. Raw PAT tokens: for scripted / CI flows that can't use the
//     install-command shape.
//
// Every table row shows derived status, not a mutable column, so
// refreshing the view always reflects what's true.
export default function EnrollmentPage() {
    const project = useCurrentProject();
    const [tab, setTab] = useState<"install" | "tokens">("install");

    return (
        <>
            <PageHeader
                title="Enrollment"
                subtitle="Generate one-shot install commands (recommended) or raw access tokens for CI / automation"
            />
            <Tabs
                value={tab}
                onValueChange={(v) => setTab(v as "install" | "tokens")}
                className="px-8 pt-4"
            >
                <TabsList>
                    <TabsTrigger value="install">
                        <Zap className="size-3.5" />
                        Install commands
                    </TabsTrigger>
                    <TabsTrigger value="tokens">Access tokens (PAT)</TabsTrigger>
                </TabsList>
                <TabsContent value="install" className="mt-4">
                    <InstallPanel projectID={project.id} />
                </TabsContent>
                <TabsContent value="tokens" className="mt-4">
                    <PATPanel projectID={project.id} />
                </TabsContent>
            </Tabs>
        </>
    );
}

// --- Install commands tab ---------------------------------------------

function InstallPanel({ projectID }: { projectID: string }) {
    const [rows, setRows] = useState<InstallArtifactListItem[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [filter, setFilter] = useState<"active" | "all">("active");
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssueInstallResponse | null>(null);
    const [pendingRevoke, setPendingRevoke] = useState<InstallArtifactListItem | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listInstallArtifacts(projectID, filter === "all");
            setRows(data);
            setError(null);
        } catch (e) {
            setError(String(e));
            toast.error(`list install artifacts: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [projectID, filter]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function confirmRevoke() {
        if (!pendingRevoke) return;
        const r = pendingRevoke;
        setPendingRevoke(null);
        try {
            await revokeInstallArtifact(projectID, r.download_id);
            toast.success("Install link revoked");
            refresh();
        } catch (e) {
            toast.error(`revoke: ${String(e)}`);
        }
    }

    return (
        <>
            <Toolbar
                left={
                    <ToggleGroup
                        type="single"
                        variant="outline"
                        size="sm"
                        value={filter}
                        onValueChange={(v) => {
                            if (v) setFilter(v as "active" | "all");
                        }}
                    >
                        <ToggleGroupItem value="active">Active</ToggleGroupItem>
                        <ToggleGroupItem value="all">All</ToggleGroupItem>
                    </ToggleGroup>
                }
                right={
                    <>
                        <Button size="sm" variant="outline" disabled={loading} onClick={refresh}>
                            {loading ? (
                                <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                                <RotateCw className="size-3.5" />
                            )}
                            Refresh
                        </Button>
                        <Button size="sm" onClick={() => setIssueOpen(true)}>
                            <Plus className="size-3.5" />
                            Generate install command
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
                        title="No install commands yet"
                        description="Generate one for a host in this project."
                    />
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[260px]">Download ID</TableHead>
                                <TableHead className="w-[120px]">Target</TableHead>
                                <TableHead>Server</TableHead>
                                <TableHead className="w-[110px]">Status</TableHead>
                                <TableHead className="w-[120px]">Expires</TableHead>
                                <TableHead>Consumed</TableHead>
                                <TableHead className="w-[80px]" />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((r) => (
                                <TableRow key={r.download_id}>
                                    <TableCell>
                                        <Mono>{r.download_id}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        {r.target_os || r.target_arch ? (
                                            `${r.target_os || "any"}/${r.target_arch || "any"}`
                                        ) : (
                                            <span className="text-text-muted">—</span>
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        <Mono>{r.server_endpoint}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        <StatusBadge status={r.status} />
                                    </TableCell>
                                    <TableCell>{fromNow(r.expires_at)}</TableCell>
                                    <TableCell>
                                        {r.consumed_at ? (
                                            <span>
                                                {fromNow(r.consumed_at)}
                                                {r.consumed_ip ? ` · ${r.consumed_ip}` : ""}
                                            </span>
                                        ) : (
                                            <span className="text-text-muted">—</span>
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        {!r.revoked && !r.consumed_at && (
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                                                onClick={() => setPendingRevoke(r)}
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

            <IssueInstallDialog
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
                projectID={projectID}
            />

            <IssuedInstallDialog result={lastIssued} onClose={() => setLastIssued(null)} />

            <AlertDialog
                open={pendingRevoke !== null}
                onOpenChange={(o) => !o && setPendingRevoke(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Revoke install link?</AlertDialogTitle>
                        <AlertDialogDescription>
                            The curl command will stop working immediately.
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

function IssueInstallDialog({
    open,
    onOpenChange,
    onIssued,
    projectID,
}: {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onIssued: (r: IssueInstallResponse) => void;
    projectID: string;
}) {
    const form = useForm<InstallFormValues>({
        resolver: zodResolver(installSchema),
        defaultValues: { server_endpoint: "", target_os: "", target_arch: "" },
    });

    // Pre-fill server_endpoint with the server's public_addr whenever
    // the dialog opens. Admins can still override; the common case
    // (LAN dev, single prod server) is that they accept the default.
    useEffect(() => {
        if (!open) return;
        getServerInfo()
            .then((info) => {
                if (info.public_addr) {
                    form.setValue("server_endpoint", info.public_addr);
                }
            })
            .catch(() => {
                /* best-effort — the field stays blank if /info fails */
            });
    }, [open, form]);

    async function submit(v: InstallFormValues) {
        try {
            const r = await issueInstallArtifact(projectID, v);
            onIssued(r);
            form.reset({ server_endpoint: "", target_os: "", target_arch: "" });
        } catch (e) {
            toast.error(`issue: ${String(e)}`);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[480px]">
                <DialogHeader>
                    <DialogTitle>Generate install command</DialogTitle>
                    <DialogDescription>
                        One-shot `curl ... | sh` bootstrap that issues a single-use access token on first fetch.
                    </DialogDescription>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(submit)} className="space-y-4">
                        <FormField
                            control={form.control}
                            name="server_endpoint"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Agent should dial</FormLabel>
                                    <FormControl>
                                        <Input placeholder="203.0.113.5:13337" {...field} />
                                    </FormControl>
                                    <FormDescription>
                                        host:port agents dial; defaults to this server's unified
                                        ingress address
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="target_os"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Target OS</FormLabel>
                                    <FormControl>
                                        <Input placeholder="linux (optional)" {...field} />
                                    </FormControl>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="target_arch"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Target arch</FormLabel>
                                    <FormControl>
                                        <Input placeholder="amd64 (optional)" {...field} />
                                    </FormControl>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="ttl_seconds"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Download link TTL (seconds)</FormLabel>
                                    <FormControl>
                                        <Input
                                            type="number"
                                            inputMode="numeric"
                                            placeholder="300"
                                            value={field.value ?? ""}
                                            onChange={(e) =>
                                                field.onChange(
                                                    e.target.value === ""
                                                        ? undefined
                                                        : Number(e.target.value),
                                                )
                                            }
                                            onBlur={field.onBlur}
                                            name={field.name}
                                            ref={field.ref}
                                        />
                                    </FormControl>
                                    <FormDescription>Default 300 (5 min)</FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <DialogFooter>
                            <Button
                                type="button"
                                variant="outline"
                                onClick={() => onOpenChange(false)}
                            >
                                Cancel
                            </Button>
                            <Button type="submit" disabled={form.formState.isSubmitting}>
                                {form.formState.isSubmitting && (
                                    <Loader2 className="size-3.5 animate-spin" />
                                )}
                                Generate
                            </Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}

function IssuedInstallDialog({
    result,
    onClose,
}: {
    result: IssueInstallResponse | null;
    onClose: () => void;
}) {
    async function copy() {
        if (!result) return;
        await navigator.clipboard.writeText(result.install_command);
        toast.success("Copied to clipboard");
    }

    return (
        <Dialog open={result !== null} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="sm:max-w-[640px]">
                <DialogHeader>
                    <DialogTitle>Install command generated</DialogTitle>
                    <DialogDescription>
                        This is the only time the command is shown. After closing, the server
                        discards the plaintext.
                    </DialogDescription>
                </DialogHeader>
                <div className="text-xs text-text-muted">Run on the target machine:</div>
                <pre className="rounded border border-border bg-surface p-3 font-mono text-xs break-all whitespace-pre-wrap">
                    {result?.install_command}
                </pre>
                <DialogFooter>
                    <Button variant="outline" onClick={copy}>
                        <Copy className="size-3.5" />
                        Copy command
                    </Button>
                    <Button onClick={onClose}>Done</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

// --- PAT tokens tab ---------------------------------------------------

function PATPanel({ projectID }: { projectID: string }) {
    const [rows, setRows] = useState<PATTokenListItem[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [filter, setFilter] = useState<"active" | "all">("active");
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssuePATResponse | null>(null);
    const [pendingRevoke, setPendingRevoke] = useState<PATTokenListItem | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listPATTokens(projectID, filter === "all");
            setRows(data);
            setError(null);
        } catch (e) {
            setError(String(e));
            toast.error(`list tokens: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [projectID, filter]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function confirmRevoke() {
        if (!pendingRevoke) return;
        const r = pendingRevoke;
        setPendingRevoke(null);
        try {
            await revokePAT(projectID, r.token_id);
            toast.success("Access token revoked");
            refresh();
        } catch (e) {
            toast.error(`revoke: ${String(e)}`);
        }
    }

    return (
        <>
            <Toolbar
                left={
                    <ToggleGroup
                        type="single"
                        variant="outline"
                        size="sm"
                        value={filter}
                        onValueChange={(v) => {
                            if (v) setFilter(v as "active" | "all");
                        }}
                    >
                        <ToggleGroupItem value="active">Active</ToggleGroupItem>
                        <ToggleGroupItem value="all">All</ToggleGroupItem>
                    </ToggleGroup>
                }
                right={
                    <>
                        <Button size="sm" variant="outline" disabled={loading} onClick={refresh}>
                            {loading ? (
                                <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                                <RotateCw className="size-3.5" />
                            )}
                            Refresh
                        </Button>
                        <Button size="sm" onClick={() => setIssueOpen(true)}>
                            <Plus className="size-3.5" />
                            Issue access token
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
                        title="No access tokens issued yet"
                        description="Prefer the install command tab unless you need raw access tokens (PATs) for a CI pipeline."
                    />
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[260px]">Token ID</TableHead>
                                <TableHead>Description</TableHead>
                                <TableHead className="w-[110px]">Status</TableHead>
                                <TableHead className="w-[80px]">Uses</TableHead>
                                <TableHead className="w-[120px]">Expires</TableHead>
                                <TableHead className="w-[80px]" />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((r) => (
                                <TableRow key={r.token_id}>
                                    <TableCell>
                                        <Mono>{r.token_id}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        {r.description || (
                                            <span className="text-text-muted">—</span>
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        <StatusBadge status={r.status} />
                                    </TableCell>
                                    <TableCell>
                                        {r.uses}/{r.max_uses}
                                    </TableCell>
                                    <TableCell>{fromNow(r.expires_at)}</TableCell>
                                    <TableCell>
                                        {!r.revoked && r.status === "pending" && (
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                                                onClick={() => setPendingRevoke(r)}
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

            <IssuePATDialog
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
                projectID={projectID}
            />
            <IssuedPATDialog result={lastIssued} onClose={() => setLastIssued(null)} />

            <AlertDialog
                open={pendingRevoke !== null}
                onOpenChange={(o) => !o && setPendingRevoke(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Revoke access token?</AlertDialogTitle>
                        <AlertDialogDescription>
                            The token will be rejected on any subsequent enrollment attempt.
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

function IssuePATDialog({
    open,
    onOpenChange,
    onIssued,
    projectID,
}: {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onIssued: (r: IssuePATResponse) => void;
    projectID: string;
}) {
    const form = useForm<PATFormValues>({
        resolver: zodResolver(patSchema),
        defaultValues: { description: "", binding_machine_id: "" },
    });

    async function submit(v: PATFormValues) {
        try {
            const r = await issuePAT(projectID, v);
            onIssued(r);
            form.reset({ description: "", binding_machine_id: "" });
        } catch (e) {
            toast.error(`issue: ${String(e)}`);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[480px]">
                <DialogHeader>
                    <DialogTitle>Issue an access token</DialogTitle>
                    <DialogDescription>
                        Raw access token (PAT) for scripted enrollment flows.
                    </DialogDescription>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(submit)} className="space-y-4">
                        <FormField
                            control={form.control}
                            name="description"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Description</FormLabel>
                                    <FormControl>
                                        <Input placeholder="Deploy for web-01" {...field} />
                                    </FormControl>
                                    <FormDescription>
                                        Free-form note shown in the list.
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="ttl_seconds"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>TTL (seconds)</FormLabel>
                                    <FormControl>
                                        <Input
                                            type="number"
                                            inputMode="numeric"
                                            placeholder="3600"
                                            value={field.value ?? ""}
                                            onChange={(e) =>
                                                field.onChange(
                                                    e.target.value === ""
                                                        ? undefined
                                                        : Number(e.target.value),
                                                )
                                            }
                                            onBlur={field.onBlur}
                                            name={field.name}
                                            ref={field.ref}
                                        />
                                    </FormControl>
                                    <FormDescription>Default 3600 (1h).</FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="max_uses"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Max uses</FormLabel>
                                    <FormControl>
                                        <Input
                                            type="number"
                                            inputMode="numeric"
                                            placeholder="1"
                                            value={field.value ?? ""}
                                            onChange={(e) =>
                                                field.onChange(
                                                    e.target.value === ""
                                                        ? undefined
                                                        : Number(e.target.value),
                                                )
                                            }
                                            onBlur={field.onBlur}
                                            name={field.name}
                                            ref={field.ref}
                                        />
                                    </FormControl>
                                    <FormDescription>Default 1 (single-use).</FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="binding_machine_id"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Binding machine ID</FormLabel>
                                    <FormControl>
                                        <Input placeholder="(optional)" {...field} />
                                    </FormControl>
                                    <FormDescription>
                                        If set, the access token is only accepted from a machine
                                        whose /etc/machine-id matches.
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <DialogFooter>
                            <Button
                                type="button"
                                variant="outline"
                                onClick={() => onOpenChange(false)}
                            >
                                Cancel
                            </Button>
                            <Button type="submit" disabled={form.formState.isSubmitting}>
                                {form.formState.isSubmitting && (
                                    <Loader2 className="size-3.5 animate-spin" />
                                )}
                                Issue
                            </Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}

function IssuedPATDialog({
    result,
    onClose,
}: {
    result: IssuePATResponse | null;
    onClose: () => void;
}) {
    async function copy() {
        if (!result) return;
        await navigator.clipboard.writeText(result.token);
        toast.success("Copied to clipboard");
    }

    return (
        <Dialog open={result !== null} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="sm:max-w-[640px]">
                <DialogHeader>
                    <DialogTitle>Access token issued</DialogTitle>
                    <DialogDescription>
                        This is the only time the token is shown. Copy it now — the server cannot
                        show it again.
                    </DialogDescription>
                </DialogHeader>
                <pre className="rounded border border-border bg-surface p-3 font-mono text-xs break-all whitespace-pre-wrap">
                    {result?.token}
                </pre>
                <DialogFooter>
                    <Button variant="outline" onClick={copy}>
                        <Copy className="size-3.5" />
                        Copy token
                    </Button>
                    <Button onClick={onClose}>Done</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

// --- Shared bits -----------------------------------------------------

type EnrollmentStatus = "pending" | "consumed" | "expired" | "revoked";

const STATUS_TONE: Record<EnrollmentStatus, "success" | "info" | "warning" | "danger"> = {
    pending: "success",
    consumed: "info",
    expired: "warning",
    revoked: "danger",
};

function StatusBadge({ status }: { status: EnrollmentStatus }) {
    return <StatusPill tone={STATUS_TONE[status]}>{status}</StatusPill>;
}
