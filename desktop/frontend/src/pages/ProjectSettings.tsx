import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Trash2 } from "lucide-react";
import { toast } from "sonner";

import Card from "../components/Card";
import DataList from "../components/DataList";
import Mono from "../components/Mono";
import PageShell from "../components/PageShell";
import { useCurrentProject, useShell } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { deleteProject } from "../lib/api";
import { humanizeError } from "../lib/humanizeError";

import { Button } from "@/components/ui/button";
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

// ProjectSettings hosts the *project-scoped, server-side* settings:
// identity (read-only here) + danger zone. Browser-local preferences
// (UI density, terminal font, default Fleet view, …) used to share
// this page via tabs but they're now at /preferences — moving them
// resolved the "Project · {slug}" subtitle lying about prefs that
// were actually browser-scoped.
//
// Re-introduce a TabsList here only when a second project-scoped
// section lands (webhooks, audit retention, etc.). One section reads
// cleanest as a tab-less stack.
export default function ProjectSettings() {
    const project = useCurrentProject();
    const { refresh } = useShell();
    const navigate = useNavigate();
    const [deleting, setDeleting] = useState(false);
    const [confirmOpen, setConfirmOpen] = useState(false);

    const handleDelete = async () => {
        setDeleting(true);
        try {
            await deleteProject(project.id);
            toast.success(`Deleted project ${project.slug}`);
            await refresh();
            navigate("/projects", { replace: true });
        } catch (e) {
            toast.error(`delete: ${humanizeError(e)}`);
            setDeleting(false);
            setConfirmOpen(false);
        }
    };

    return (
        <>
        <PageShell title="Settings" subtitle={`Project · ${project.slug}`} bodyPadding={8}>
                <div
                    style={{
                        display: "flex",
                        flexDirection: "column",
                        gap: space[4],
                        maxWidth: 720,
                    }}
                >
                    <Card header="Identity" padding={5}>
                        <DataList
                            items={[
                                { label: "name", value: project.name },
                                { label: "slug", value: <Mono>{project.slug}</Mono> },
                                { label: "id", value: <Mono size={11}>{project.id}</Mono> },
                            ]}
                        />
                    </Card>

                    <Card
                        header={
                            <span style={{ color: palette.danger }}>Danger zone</span>
                        }
                        padding={5}
                    >
                        <div
                            style={{
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "space-between",
                                gap: space[4],
                            }}
                        >
                            <div
                                style={{
                                    color: palette.textSecondary,
                                    fontSize: 13,
                                    lineHeight: 1.5,
                                }}
                            >
                                Deleting a project removes its hosts, sessions, tokens,
                                and install artifacts. This cannot be undone.
                            </div>
                            <Button
                                variant="destructive"
                                size="sm"
                                onClick={() => setConfirmOpen(true)}
                                disabled={deleting}
                            >
                                {deleting ? (
                                    <Loader2 className="size-3.5 animate-spin" />
                                ) : (
                                    <Trash2 className="size-3.5" />
                                )}
                                Delete project
                            </Button>
                        </div>
                    </Card>
                </div>
        </PageShell>

            <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Delete this project?</AlertDialogTitle>
                        <AlertDialogDescription>
                            <span>Deleting </span>
                            <Mono>{project.slug}</Mono>
                            <span>
                                {" "}
                                permanently removes every host, session, token, and
                                install artifact inside it. This cannot be undone.
                            </span>
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={(e) => {
                                e.preventDefault();
                                void handleDelete();
                            }}
                            disabled={deleting}
                        >
                            {deleting && <Loader2 className="size-3.5 animate-spin" />}
                            Delete
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}
