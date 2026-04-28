import * as React from "react";
import { GripVertical } from "lucide-react";
import {
    Group,
    Panel,
    Separator as PanelSeparator,
    type GroupProps,
    type Layout,
} from "react-resizable-panels";

import { cn } from "@/lib/cn";

const STORAGE_PREFIX = "platypus.layout.";

// useLayoutStorage hands the wrapper a localStorage-backed layout
// roundtripper. v4 of react-resizable-panels dropped the autoSaveId
// shorthand, so we plumb defaultLayout/onLayoutChanged ourselves and
// keep the call sites looking the same as the shadcn pattern.
function useLayoutStorage(autoSaveId?: string) {
    const initial = React.useMemo<Layout | undefined>(() => {
        if (!autoSaveId || typeof window === "undefined") return undefined;
        try {
            const raw = window.localStorage.getItem(`${STORAGE_PREFIX}${autoSaveId}`);
            if (!raw) return undefined;
            const parsed = JSON.parse(raw);
            return parsed && typeof parsed === "object" ? (parsed as Layout) : undefined;
        } catch {
            return undefined;
        }
    }, [autoSaveId]);

    const persist = React.useCallback(
        (layout: Layout) => {
            if (!autoSaveId || typeof window === "undefined") return;
            try {
                window.localStorage.setItem(
                    `${STORAGE_PREFIX}${autoSaveId}`,
                    JSON.stringify(layout),
                );
            } catch {
                // best effort — quota errors etc. just lose persistence
            }
        },
        [autoSaveId],
    );

    return { initial, persist };
}

type ResizablePanelGroupProps = Omit<GroupProps, "orientation"> & {
    direction: "horizontal" | "vertical";
    autoSaveId?: string;
};

function ResizablePanelGroup({
    className,
    direction,
    autoSaveId,
    defaultLayout,
    onLayoutChanged,
    ...props
}: ResizablePanelGroupProps) {
    const storage = useLayoutStorage(autoSaveId);
    return (
        <Group
            data-slot="resizable-panel-group"
            data-panel-group-direction={direction}
            orientation={direction}
            defaultLayout={storage.initial ?? defaultLayout}
            onLayoutChanged={(layout) => {
                storage.persist(layout);
                onLayoutChanged?.(layout);
            }}
            className={cn(
                "flex h-full w-full data-[panel-group-direction=vertical]:flex-col",
                className,
            )}
            {...props}
        />
    );
}

const ResizablePanel = Panel;

function ResizableHandle({
    withHandle,
    className,
    ...props
}: React.ComponentProps<typeof PanelSeparator> & { withHandle?: boolean }) {
    return (
        <PanelSeparator
            data-slot="resizable-handle"
            className={cn(
                "relative flex w-px shrink-0 items-center justify-center bg-border",
                "after:absolute after:inset-y-0 after:left-1/2 after:w-1.5 after:-translate-x-1/2",
                "hover:bg-primary/40 data-[resize-handle-active=true]:bg-primary",
                "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-offset-1",
                "[[data-panel-group-direction=vertical]_&]:h-px [[data-panel-group-direction=vertical]_&]:w-full",
                "[[data-panel-group-direction=vertical]_&]:after:left-0 [[data-panel-group-direction=vertical]_&]:after:h-1.5 [[data-panel-group-direction=vertical]_&]:after:w-full [[data-panel-group-direction=vertical]_&]:after:-translate-y-1/2 [[data-panel-group-direction=vertical]_&]:after:translate-x-0",
                className,
            )}
            {...props}
        >
            {withHandle && (
                <div className="z-10 flex h-4 w-3 items-center justify-center rounded-sm border bg-border">
                    <GripVertical className="size-2.5" />
                </div>
            )}
        </PanelSeparator>
    );
}

export {
    type PanelImperativeHandle,
    usePanelRef,
} from "react-resizable-panels";

export { ResizablePanelGroup, ResizablePanel, ResizableHandle };
