import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2, Plus, Router, RotateCw, Search } from "lucide-react";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";
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
import { Listener, createListener, deleteListener, listListeners } from "../lib/api";
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

// Zod schema for the create-listener form. Lives next to the page
// because the fields / bounds are specific to listeners; if we grow
// more forms with the same port validation we'll hoist it.
const listenerSchema = z.object({
    host: z.string().min(1, "host is required"),
    port: z
        .number({ message: "port is required" })
        .int("port must be an integer")
        .min(1, "port must be ≥ 1")
        .max(65535, "port must be ≤ 65535"),
});
type ListenerFormValues = z.infer<typeof listenerSchema>;

// ListenersPage is the cross-listener list at /projects/:slug/listeners.
// Always-visible "+ New listener" button in PageHeader actions solves
// the IA gap where there was no entry point to create one.
export default function ListenersPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const [listeners, setListeners] = useState<Listener[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [createOpen, setCreateOpen] = useState(false);
    const [pendingDelete, setPendingDelete] = useState<Listener | null>(null);
    const [query, setQuery] = useState("");

    const createForm = useForm<ListenerFormValues>({
        resolver: zodResolver(listenerSchema),
        defaultValues: { host: "0.0.0.0", port: 13337 },
    });

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setListeners(await listListeners(project.id));
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

    async function handleCreate(v: ListenerFormValues) {
        try {
            const l = await createListener(project.id, v.host, v.port);
            toast.success(`Listener ${l.host}:${l.port} created`);
            setCreateOpen(false);
            createForm.reset({ host: "0.0.0.0", port: 13337 });
            await refresh();
            navigate(`/projects/${project.slug}/listeners/${l.id}`);
        } catch (e) {
            toast.error(`create: ${String(e)}`);
        }
    }

    async function confirmDelete() {
        if (!pendingDelete) return;
        const l = pendingDelete;
        setPendingDelete(null);
        try {
            await deleteListener(project.id, l.id);
            toast.success("Listener stopped");
            refresh();
        } catch (e) {
            toast.error(`delete: ${String(e)}`);
        }
    }

    const filtered = useMemo(() => {
        if (!listeners) return null;
        const q = query.trim().toLowerCase();
        if (!q) return listeners;
        return listeners.filter((l) =>
            `${l.host}:${l.port} ${l.public_ip ?? ""}`.toLowerCase().includes(q),
        );
    }, [listeners, query]);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Listeners"
                subtitle={listeners === null ? "Loading…" : `${listeners.length} total`}
                actions={
                    <>
                        <Button size="sm" variant="outline" disabled={loading} onClick={refresh}>
                            {loading ? (
                                <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                                <RotateCw className="size-3.5" />
                            )}
                            Refresh
                        </Button>
                        <Button size="sm" onClick={() => setCreateOpen(true)}>
                            <Plus className="size-3.5" />
                            New listener
                        </Button>
                    </>
                }
            />
            <Toolbar
                left={
                    <div className="relative max-w-[360px] w-full">
                        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-text-muted pointer-events-none" />
                        <Input
                            placeholder="Search host:port or public IP"
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            className="h-8 pl-8"
                        />
                    </div>
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
                {!listeners && (
                    <div className="flex items-center justify-center p-20">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                )}
                {listeners && listeners.length === 0 && (
                    <EmptyState
                        icon={<Router className="size-5" />}
                        title="No listeners yet"
                        description="Create a listener to start accepting agent connections. Each listener binds to a host:port and stays running until you stop it."
                        action={
                            <Button onClick={() => setCreateOpen(true)}>
                                <Plus className="size-3.5" />
                                New listener
                            </Button>
                        }
                    />
                )}
                {filtered && filtered.length === 0 && listeners && listeners.length > 0 && (
                    <EmptyState
                        title="No matches"
                        description={`No listener matches "${query}".`}
                    />
                )}
                {filtered && filtered.length > 0 && (
                    <Card padding={0}>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Endpoint</TableHead>
                                    <TableHead>Public IP</TableHead>
                                    <TableHead className="w-[160px]">Created</TableHead>
                                    <TableHead className="w-[120px]">Status</TableHead>
                                    <TableHead className="w-[80px]" />
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {filtered.map((l) => (
                                    <TableRow
                                        key={l.id}
                                        className="cursor-pointer"
                                        onClick={() =>
                                            navigate(
                                                `/projects/${project.slug}/listeners/${l.id}`,
                                            )
                                        }
                                    >
                                        <TableCell>
                                            <Mono>{`${l.host}:${l.port}`}</Mono>
                                        </TableCell>
                                        <TableCell>
                                            {l.public_ip ? (
                                                <Mono>{l.public_ip}</Mono>
                                            ) : (
                                                <span className="text-text-muted">—</span>
                                            )}
                                        </TableCell>
                                        <TableCell className="text-text-secondary">
                                            {fromNow(l.created_at)}
                                        </TableCell>
                                        <TableCell>
                                            <StatusPill tone="success">listening</StatusPill>
                                        </TableCell>
                                        <TableCell>
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                                                onClick={(e) => {
                                                    e.stopPropagation();
                                                    setPendingDelete(l);
                                                }}
                                            >
                                                Stop
                                            </Button>
                                        </TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                    </Card>
                )}
            </div>

            {/* Create dialog */}
            <Dialog open={createOpen} onOpenChange={setCreateOpen}>
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>New listener</DialogTitle>
                        <DialogDescription>
                            Binds a TLS port on the server for agents to dial in.
                        </DialogDescription>
                    </DialogHeader>
                    <Form {...createForm}>
                        <form
                            onSubmit={createForm.handleSubmit(handleCreate)}
                            className="space-y-4"
                        >
                            <FormField
                                control={createForm.control}
                                name="host"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Host</FormLabel>
                                        <FormControl>
                                            <Input placeholder="0.0.0.0" {...field} />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={createForm.control}
                                name="port"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Port</FormLabel>
                                        <FormControl>
                                            <Input
                                                type="number"
                                                inputMode="numeric"
                                                min={1}
                                                max={65535}
                                                value={field.value ?? ""}
                                                onChange={(e) => {
                                                    const v = e.target.value;
                                                    field.onChange(v === "" ? undefined : Number(v));
                                                }}
                                                onBlur={field.onBlur}
                                                name={field.name}
                                                ref={field.ref}
                                            />
                                        </FormControl>
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

            {/* Delete confirmation */}
            <AlertDialog
                open={pendingDelete !== null}
                onOpenChange={(o) => !o && setPendingDelete(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            Stop listener {pendingDelete?.host}:{pendingDelete?.port}?
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                            Existing sessions stay alive, but no new connections will be
                            accepted. The row is removed from storage.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={confirmDelete}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Stop
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </div>
    );
}
