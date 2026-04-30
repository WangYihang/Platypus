import { useState } from "react";
import { Plus } from "lucide-react";
import { DndContext, DragEndEvent } from "@dnd-kit/core";
import {
    SortableContext,
    arrayMove,
    verticalListSortingStrategy,
} from "@dnd-kit/sortable";

import EmptyState from "../components/EmptyState";
import PageShell from "../components/PageShell";
import StatusPills from "../components/StatusPills";
import AddServerDialog from "../layout/AddServerDialog";
import SortableServerRow from "../layout/server-switcher/SortableServerRow";
import {
    useActiveServerId,
    useServerList,
} from "../layout/server-switcher/hooks";
import { switchServer } from "../lib/auth";
import {
    ServerProfile,
    reorderServers,
} from "../lib/servers";
import { useDragSensors } from "../lib/dnd";
import { palette } from "../layout/theme";

import { Button } from "@/components/ui/button";

// Servers is the global "/servers" page — promoted from the legacy
// ManageServersDialog so the same surface bookmarks, deep-links, and
// fits the Vercel-style flat top-bar IA. The page reuses
// SortableServerRow (drag-to-reorder, rename, sign-out, remove) so the
// dropdown switcher and this page stay visually identical.
//
// Activation differs from the dropdown: clicking a row here switches
// the active server in-place. It does *not* navigate away — the user
// is on /servers, which is a global route, so the switch only updates
// connection state. If the user isn't logged in on the target server
// we redirect to /login like the dropdown does.
export default function Servers() {
    const profiles = useServerList();
    const activeId = useActiveServerId();
    const sensors = useDragSensors();
    const [addOpen, setAddOpen] = useState(false);

    const handleDragEnd = (e: DragEndEvent) => {
        const { active: from, over } = e;
        if (!over || from.id === over.id) return;
        const ids = profiles.map((p) => p.id);
        const fromIdx = ids.indexOf(String(from.id));
        const toIdx = ids.indexOf(String(over.id));
        if (fromIdx < 0 || toIdx < 0) return;
        reorderServers(arrayMove(ids, fromIdx, toIdx));
    };

    const onActivate = (profile: ServerProfile) => {
        // Fire-and-forget: switchServer updates the active session
        // pool; if a refresh token is missing the user lands on
        // /login via the auth handler. We don't navigate from here.
        void switchServer(profile.id);
    };

    return (
        <PageShell
            title="Servers"
            subtitle="Manage the Platypus servers saved in this client"
            pills={
                <StatusPills
                    pills={[
                        { tone: "info", count: profiles.length, label: "saved" },
                    ]}
                />
            }
            actions={
                <Button size="sm" onClick={() => setAddOpen(true)}>
                    <Plus className="size-3.5" />
                    Add server
                </Button>
            }
        >
            {profiles.length === 0 ? (
                <EmptyState
                    title="No servers saved"
                    description="Add the URL of a Platypus server to get started. Saved profiles persist locally and let you flip between servers from the top-bar switcher."
                    fill
                    action={
                        <Button onClick={() => setAddOpen(true)}>
                            <Plus className="size-3.5" />
                            Add server
                        </Button>
                    }
                />
            ) : (
                <div
                    style={{
                        display: "flex",
                        flexDirection: "column",
                        gap: 4,
                        background: palette.surface,
                        border: `1px solid ${palette.border}`,
                        borderRadius: 8,
                        padding: 6,
                    }}
                >
                    <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
                        <SortableContext
                            items={profiles.map((p) => p.id)}
                            strategy={verticalListSortingStrategy}
                        >
                            {profiles.map((p, i) => (
                                <SortableServerRow
                                    key={p.id}
                                    profile={p}
                                    index={i}
                                    active={p.id === activeId}
                                    onActivate={() => onActivate(p)}
                                    onCloseMenu={() => {}}
                                />
                            ))}
                        </SortableContext>
                    </DndContext>
                </div>
            )}
            <AddServerDialog open={addOpen} onOpenChange={setAddOpen} />
        </PageShell>
    );
}
