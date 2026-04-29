import { useDroppable } from "@dnd-kit/core";

import { cn } from "@/lib/cn";

interface Props {
    path: string;
    label: string;
    onClick: () => void;
    isLast: boolean;
}

// Each breadcrumb segment doubles as a drop target so a dragged file can be moved
// to any ancestor directory.
export default function Crumb({ path, label, onClick, isLast }: Props) {
    const { setNodeRef, isOver } = useDroppable({
        id: `crumb:${path}`,
        data: { dirName: label, isDir: true, fullPath: path, isCrumb: true },
    });
    return (
        <button
            ref={setNodeRef}
            type="button"
            onClick={onClick}
            className={cn(
                "font-mono text-sm hover:underline",
                isLast ? "text-foreground" : "text-muted-foreground",
                isOver && "rounded bg-accent px-1",
            )}
        >
            {label}
        </button>
    );
}
