import { useCallback, useEffect, useState } from "react";
import { Copy, Loader2, Plus, RotateCw, Trash2, Zap } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { UseFormReturn, useForm } from "react-hook-form";
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
    InstallPlatform,
    IssueInstallResponse,
    IssueEnrollmentTokenResponse,
    EnrollmentTokenListItem,
    getServerInfo,
    issueInstallArtifact,
    issueEnrollmentToken,
    listInstallArtifacts,
    listInstallPlatforms,
    listEnrollmentTokens,
    revokeInstallArtifact,
    revokeEnrollmentToken,
} from "../lib/api";
import { formatSeconds, fromNow } from "../lib/time";
import { humanizeError } from "../lib/humanizeError";
import {
    EnrollmentStatus,
    STATUS_LABEL,
    STATUS_TONE,
} from "./enrollment/status";

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

// PlatformsState tracks the install-target picker's lifecycle so the UI
// can disable the dropdown while loading and surface the right empty /
// error hint without conflating "no manifest published" with "request
// failed".
type PlatformsState =
    | { status: "loading" }
    | { status: "ready"; platforms: InstallPlatform[]; channel: string }
    | { status: "empty"; channel: string }
    | { status: "error"; message: string };

// Display order for the install-target picker. OSes a deployer is most
// likely to pick come first; within an OS the common 64-bit archs lead
// and the long tail (mips, riscv, …) trails. Anything not in the list
// gets sorted alphabetically and appended — keeps us forward-compatible
// with future GOOS/GOARCH additions without code changes.
const OS_ORDER = ["linux", "darwin", "windows", "freebsd", "openbsd", "netbsd"];
const ARCH_ORDER = [
    "amd64",
    "arm64",
    "arm",
    "386",
    "riscv64",
    "ppc64le",
    "s390x",
    "loong64",
    "mips64le",
    "mips64",
    "mipsle",
    "mips",
];

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

// EnrollmentPage bundles two related admin surfaces onto one page
// because they share the same mental model — "hand an agent something
// short-lived so it can join the fleet":
//
//  1. Install commands: one-shot `curl ... | sh` bootstraps that
//     atomically issue an enrollment token the first time they're
//     fetched.
//  2. Raw enrollment tokens: for scripted / CI flows that can't use
//     the install-command shape.
//
// The tokens are called "enrollment tokens" on the user surface —
// the backend tables and code paths still use the historical "PAT"
// name, but exposing that acronym led users to confuse them with
// account-scoped API tokens. They are not. They burn on first
// enrollment; mTLS takes over for everything after.
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
                subtitle="Generate one-shot install commands (recommended) or raw enrollment tokens for CI / automation"
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
                    <TabsTrigger value="tokens">Enrollment tokens</TabsTrigger>
                </TabsList>
                <TabsContent value="install" className="mt-4">
                    <InstallPanel projectID={project.id} projectSlug={project.slug} />
                </TabsContent>
                <TabsContent value="tokens" className="mt-4">
                    <PATPanel projectID={project.id} />
                </TabsContent>
            </Tabs>
        </>
    );
}

// --- Install commands tab ---------------------------------------------

function InstallPanel({
    projectID,
    projectSlug,
}: {
    projectID: string;
    projectSlug: string;
}) {
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
            toast.error(`Couldn't load install commands: ${humanizeError(e)}`);
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
            toast.error(`Couldn't revoke: ${humanizeError(e)}`);
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

            <IssuedInstallDialog
                result={lastIssued}
                projectSlug={projectSlug}
                onClose={() => setLastIssued(null)}
            />

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

// preferredOrder returns a comparator that ranks `priority` items first
// (in their declared order) and trails everything else alphabetically.
// Used to bubble the densest-used OSes / archs to the front of the
// pickers without dropping forward compatibility for new GOOS/GOARCH
// values that ship in future manifests.
function preferredOrder(priority: string[]): (a: string, b: string) => number {
    return (a, b) => {
        const ia = priority.indexOf(a);
        const ib = priority.indexOf(b);
        if (ia === -1 && ib === -1) return a.localeCompare(b);
        if (ia === -1) return 1;
        if (ib === -1) return -1;
        return ia - ib;
    };
}

// PlatformPickerField is the "Target platform" form control. Two
// cascading ToggleGroups: pick an OS, then the matching archs unfold;
// leaving both unselected means "auto-detect at runtime", which keeps
// today's "blank target_os + blank target_arch → install one-liner
// self-detects" wire contract. This shape avoids Radix Select's
// no-empty-value rule (no sentinel needed — ToggleGroup type="single"
// represents the unselected state with an empty string natively) and
// matches how an admin actually thinks: first decide the OS, then pick
// the architecture from the options that platform supports.
function PlatformPickerField({
    form,
    platforms,
}: {
    form: UseFormReturn<InstallFormValues>;
    platforms: PlatformsState;
}) {
    const targetOS = form.watch("target_os") ?? "";
    const targetArch = form.watch("target_arch") ?? "";

    // Index live platforms by OS so the arch row only shows architectures
    // that actually have a published binary on the selected OS.
    const archsByOS = new Map<string, string[]>();
    if (platforms.status === "ready") {
        const tmp = new Map<string, Set<string>>();
        for (const p of platforms.platforms) {
            if (!tmp.has(p.os)) tmp.set(p.os, new Set());
            tmp.get(p.os)!.add(p.arch);
        }
        for (const [os, set] of tmp) {
            archsByOS.set(os, [...set].sort(preferredOrder(ARCH_ORDER)));
        }
    }
    const osList = [...archsByOS.keys()].sort(preferredOrder(OS_ORDER));
    const archList = targetOS ? (archsByOS.get(targetOS) ?? []) : [];

    function pickOS(next: string) {
        // Radix emits "" when the user deselects the active item by
        // clicking it again — let that propagate as "back to
        // auto-detect" rather than an "OS but no arch" half state.
        form.setValue("target_os", next);
        // Switching OS invalidates whatever arch was picked — even if
        // the new OS happens to have the same arch name, forcing a
        // re-pick keeps the cascade honest.
        form.setValue("target_arch", "");
    }

    function pickArch(next: string) {
        form.setValue("target_arch", next);
    }

    const description = (() => {
        if (platforms.status === "loading") return "Loading platforms…";
        if (platforms.status === "empty") {
            return `No agent binaries on channel "${platforms.channel}" yet — run the agent-publisher sidecar (or seed MinIO) to populate this picker. The install command still works (auto-detect at runtime).`;
        }
        if (platforms.status === "error") {
            return `Couldn't load platforms: ${platforms.message}. The install command still works (auto-detect at runtime).`;
        }
        if (targetOS && targetArch) {
            return `Pinned to ${targetOS}/${targetArch}.`;
        }
        if (targetOS) {
            return `Pick an architecture, or leave the OS unselected to auto-detect at runtime.`;
        }
        return "Leave both unselected for the install one-liner to auto-detect, or pick an OS to start narrowing.";
    })();

    return (
        <FormItem>
            <FormLabel>Target platform</FormLabel>
            <div className="space-y-3">
                <ToggleGroup
                    type="single"
                    variant="outline"
                    size="sm"
                    value={targetOS}
                    onValueChange={pickOS}
                    disabled={platforms.status !== "ready"}
                    className="flex-wrap justify-start"
                >
                    {osList.map((os) => (
                        <ToggleGroupItem key={os} value={os}>
                            {os}
                        </ToggleGroupItem>
                    ))}
                </ToggleGroup>
                {targetOS && archList.length > 0 && (
                    <ToggleGroup
                        type="single"
                        variant="outline"
                        size="sm"
                        value={targetArch}
                        onValueChange={pickArch}
                        className="flex-wrap justify-start"
                    >
                        {archList.map((arch) => (
                            <ToggleGroupItem key={arch} value={arch}>
                                {arch}
                            </ToggleGroupItem>
                        ))}
                    </ToggleGroup>
                )}
            </div>
            <FormDescription>{description}</FormDescription>
            <FormMessage />
        </FormItem>
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
        // onBlur revalidation surfaces "must be a positive integer"
        // immediately after the user leaves the TTL field instead of
        // waiting for submit — matters most for the numeric inputs
        // (ttl_seconds, max_uses) where a typo is silently accepted
        // until the dialog is dismissed.
        mode: "onBlur",
        defaultValues: { server_endpoint: "", target_os: "", target_arch: "" },
    });

    // Live (os, arch) list from the active channel's manifest. Drives
    // the platform picker — admins can only choose targets the
    // distributor can actually serve, and the empty-state hint points
    // them at the publisher when the channel hasn't been seeded yet.
    const [platforms, setPlatforms] = useState<PlatformsState>({ status: "loading" });

    // Pre-fill server_endpoint with the server's public_addr and load
    // the platform list whenever the dialog opens. Both are best-effort
    // — failures fall back to a usable empty state.
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
        setPlatforms({ status: "loading" });
        listInstallPlatforms()
            .then((r) => {
                if (r.platforms.length === 0) {
                    setPlatforms({ status: "empty", channel: r.channel });
                } else {
                    setPlatforms({
                        status: "ready",
                        platforms: r.platforms,
                        channel: r.channel,
                    });
                }
            })
            .catch((e) => {
                setPlatforms({ status: "error", message: humanizeError(e) });
            });
    }, [open, form]);

    async function submit(v: InstallFormValues) {
        try {
            const r = await issueInstallArtifact(projectID, v);
            onIssued(r);
            form.reset({ server_endpoint: "", target_os: "", target_arch: "" });
        } catch (e) {
            toast.error(`Couldn't generate install command: ${humanizeError(e)}`);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[480px]">
                <DialogHeader>
                    <DialogTitle>Generate install command</DialogTitle>
                    <DialogDescription>
                        One-shot `curl ... | sh` bootstrap that issues a single-use enrollment token on first fetch.
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
                                        <Input
                                            autoFocus
                                            placeholder="203.0.113.5:13337"
                                            {...field}
                                        />
                                    </FormControl>
                                    <FormDescription>
                                        host:port agents dial; defaults to this server's unified
                                        ingress address
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <PlatformPickerField form={form} platforms={platforms} />
                        <FormField
                            control={form.control}
                            name="ttl_seconds"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Expires in (seconds)</FormLabel>
                                    <FormControl>
                                        <Input
                                            type="number"
                                            inputMode="numeric"
                                            placeholder="300 (= 5m)"
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
                                    <FormDescription>
                                        How long the install URL stays valid.{" "}
                                        {typeof field.value === "number" && field.value > 0
                                            ? `= ${formatSeconds(field.value)}`
                                            : "Default 300 (= 5m)."}
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
    projectSlug,
    onClose,
}: {
    result: IssueInstallResponse | null;
    projectSlug: string;
    onClose: () => void;
}) {
    const navigate = useNavigate();

    async function copy() {
        if (!result) return;
        await navigator.clipboard.writeText(result.install_command);
        toast.success("Copied to clipboard");
    }

    function done() {
        // After Done, drop the user on /fleet with await=enroll so
        // the EnrollmentWaitBanner mounts: it polls listHosts every
        // 3s and switches into a green "agent enrolled" state once
        // the new host dials back. Without this navigation the user
        // copied the command and then sat on a static page wondering
        // if anything was happening.
        onClose();
        navigate(`/projects/${projectSlug}/fleet?await=enroll`);
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
                <div className="text-xs text-text-muted">
                    After running it on the host, return to Fleet — new agents appear there
                    automatically within a few seconds.
                </div>
                <DialogFooter>
                    <Button variant="outline" onClick={copy}>
                        <Copy className="size-3.5" />
                        Copy command
                    </Button>
                    <Button onClick={done}>I'll run this — show me Fleet</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

// --- PAT tokens tab ---------------------------------------------------

function PATPanel({ projectID }: { projectID: string }) {
    const [rows, setRows] = useState<EnrollmentTokenListItem[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [filter, setFilter] = useState<"active" | "all">("active");
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssueEnrollmentTokenResponse | null>(null);
    const [pendingRevoke, setPendingRevoke] = useState<EnrollmentTokenListItem | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listEnrollmentTokens(projectID, filter === "all");
            setRows(data);
            setError(null);
        } catch (e) {
            setError(humanizeError(e));
            toast.error(`Couldn't load enrollment tokens: ${humanizeError(e)}`);
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
            await revokeEnrollmentToken(projectID, r.token_id);
            toast.success("Enrollment token revoked");
            refresh();
        } catch (e) {
            toast.error(`Couldn't revoke: ${humanizeError(e)}`);
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
                            Issue enrollment token
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
                        title="No enrollment tokens issued yet"
                        description="Prefer the install command tab unless you need raw enrollment tokens for a CI pipeline."
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
                        <AlertDialogTitle>Revoke enrollment token?</AlertDialogTitle>
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
    onIssued: (r: IssueEnrollmentTokenResponse) => void;
    projectID: string;
}) {
    const form = useForm<PATFormValues>({
        resolver: zodResolver(patSchema),
        // Same rationale as IssueInstallDialog — flag bad numeric
        // values on blur instead of waiting for submit.
        mode: "onBlur",
        defaultValues: { description: "", binding_machine_id: "" },
    });

    async function submit(v: PATFormValues) {
        try {
            const r = await issueEnrollmentToken(projectID, v);
            onIssued(r);
            form.reset({ description: "", binding_machine_id: "" });
        } catch (e) {
            toast.error(`Couldn't issue enrollment token: ${humanizeError(e)}`);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[480px]">
                <DialogHeader>
                    <DialogTitle>Issue an enrollment token</DialogTitle>
                    <DialogDescription>
                        Raw token an agent can submit at /enroll. Use this when you need
                        the bare credential for a CI / scripted flow that can't run the
                        install command shape.
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
                                        <Input
                                            autoFocus
                                            placeholder="Deploy for web-01"
                                            {...field}
                                        />
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
                                    <FormLabel>Expires in (seconds)</FormLabel>
                                    <FormControl>
                                        <Input
                                            type="number"
                                            inputMode="numeric"
                                            placeholder="3600 (= 1h)"
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
                                    <FormDescription>
                                        How long the enrollment token stays valid.{" "}
                                        {typeof field.value === "number" && field.value > 0
                                            ? `= ${formatSeconds(field.value)}`
                                            : "Default 3600 (= 1h)."}
                                    </FormDescription>
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
                                    <FormLabel>Restrict to machine</FormLabel>
                                    <FormControl>
                                        <Input
                                            placeholder="machine-id (optional)"
                                            {...field}
                                        />
                                    </FormControl>
                                    <FormDescription>
                                        Optional. Paste the host's
                                        <code> /etc/machine-id</code> contents to lock this
                                        enrollment token to that one host — useful for
                                        long-lived tokens, since a stolen token cannot be
                                        replayed elsewhere. Leave empty for short-lived
                                        tokens you don't plan to reuse.
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
    result: IssueEnrollmentTokenResponse | null;
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
                    <DialogTitle>Enrollment token issued</DialogTitle>
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
// STATUS_LABEL / STATUS_TONE / EnrollmentStatus live in
// ./enrollment/status so they're testable in isolation; this page is
// just a consumer.
function StatusBadge({ status }: { status: EnrollmentStatus }) {
    return <StatusPill tone={STATUS_TONE[status]}>{STATUS_LABEL[status]}</StatusPill>;
}
