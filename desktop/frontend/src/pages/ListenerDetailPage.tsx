import { useCallback, useEffect, useState } from "react";
import { ArrowLeft, Loader2, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useNavigate, useParams } from "react-router-dom";

import Card from "../components/Card";
import DataList from "../components/DataList";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { Listener, deleteListener, listListeners } from "../lib/api";
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

// ListenerDetailPage is /projects/:slug/listeners/:listenerId.
// Detail card + Stop action. Back button returns to the list.
export default function ListenerDetailPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { listenerId } = useParams<{ listenerId: string }>();
    const [listener, setListener] = useState<Listener | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);
    const [confirmOpen, setConfirmOpen] = useState(false);

    const refresh = useCallback(async () => {
        if (!listenerId) return;
        setLoading(true);
        try {
            const list = await listListeners(project.id);
            setListener(list.find((l) => l.id === listenerId) ?? null);
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [project.id, listenerId]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function confirmDelete() {
        if (!listener) return;
        setConfirmOpen(false);
        try {
            await deleteListener(project.id, listener.id);
            toast.success("Listener stopped");
            navigate(`/projects/${project.slug}/listeners`);
        } catch (e) {
            toast.error(`delete: ${String(e)}`);
        }
    }

    if (loading && !listener) {
        return (
            <div className="flex items-center justify-center p-20">
                <Loader2 className="size-5 animate-spin text-text-muted" />
            </div>
        );
    }
    if (error) {
        return (
            <div style={{ padding: space[5] }}>
                <div
                    style={{
                        padding: `${space[3]}px ${space[4]}px`,
                        border: `1px solid ${palette.danger}`,
                        borderRadius: 6,
                        color: palette.danger,
                        fontSize: 13,
                    }}
                >
                    {error}
                </div>
            </div>
        );
    }
    if (!listener) {
        return (
            <EmptyState
                title="Listener not found"
                description="It may have been stopped, or you may have lost access."
                fill
                action={
                    <Button onClick={() => navigate(`/projects/${project.slug}/listeners`)}>
                        Back to listeners
                    </Button>
                }
            />
        );
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title={
                    <span className="flex items-center gap-3">
                        <Button
                            size="icon"
                            variant="outline"
                            className="size-7"
                            onClick={() => navigate(`/projects/${project.slug}/listeners`)}
                        >
                            <ArrowLeft className="size-3.5" />
                        </Button>
                        <Mono size={22}>{`${listener.host}:${listener.port}`}</Mono>
                    </span>
                }
                subtitle={`listener · created ${fromNow(listener.created_at)}`}
                actions={
                    <Button
                        variant="outline"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setConfirmOpen(true)}
                    >
                        <Trash2 className="size-3.5" />
                        Stop listener
                    </Button>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                <div style={{ maxWidth: 720 }}>
                    <Card header="Listener">
                        <DataList
                            items={[
                                {
                                    label: "host:port",
                                    value: <Mono>{`${listener.host}:${listener.port}`}</Mono>,
                                },
                                {
                                    label: "public ip",
                                    value: listener.public_ip ? (
                                        <Mono>{listener.public_ip}</Mono>
                                    ) : (
                                        "—"
                                    ),
                                },
                                {
                                    label: "shell",
                                    value: <Mono>{listener.shell_path || "default"}</Mono>,
                                },
                                {
                                    label: "created",
                                    value: fromNow(listener.created_at),
                                },
                                {
                                    label: "status",
                                    value: <StatusPill tone="success">listening</StatusPill>,
                                },
                            ]}
                        />
                    </Card>
                </div>
            </div>

            <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            Stop listener {listener.host}:{listener.port}?
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
