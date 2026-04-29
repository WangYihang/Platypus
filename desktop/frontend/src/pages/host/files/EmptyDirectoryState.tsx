import { FilePlus, FolderPlus, FolderX, SearchX, Upload } from "lucide-react";

import { Button } from "@/components/ui/button";

interface Props {
    // hasFilter narrows the empty state from "this directory has no
    // files" to "your filter elided everything" — the CTA we offer
    // changes accordingly. Without this distinction operators waste
    // time wondering why a directory they know is full reads as
    // empty.
    hasFilter: boolean;
    onClearFilter: () => void;
    onNewFile: () => void;
    onNewFolder: () => void;
    onUploadHere: () => void;
}

export default function EmptyDirectoryState({
    hasFilter,
    onClearFilter,
    onNewFile,
    onNewFolder,
    onUploadHere,
}: Props) {
    if (hasFilter) {
        return (
            <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm">
                <SearchX className="size-10 text-muted-foreground/40" />
                <div>
                    <div className="font-medium">No matches</div>
                    <div className="mt-1 text-xs text-muted-foreground">
                        Your filter hid every entry in this directory.
                    </div>
                </div>
                <Button size="sm" variant="outline" onClick={onClearFilter}>
                    Clear filter
                </Button>
            </div>
        );
    }
    return (
        <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm">
            <FolderX className="size-10 text-muted-foreground/40" />
            <div>
                <div className="font-medium">Empty directory</div>
                <div className="mt-1 text-xs text-muted-foreground">
                    Drop files here, or get started below.
                </div>
            </div>
            <div className="flex flex-wrap items-center justify-center gap-2">
                <Button size="sm" variant="outline" onClick={onNewFolder}>
                    <FolderPlus className="size-3.5" />
                    New folder
                </Button>
                <Button size="sm" variant="outline" onClick={onNewFile}>
                    <FilePlus className="size-3.5" />
                    New file
                </Button>
                <Button size="sm" variant="outline" onClick={onUploadHere}>
                    <Upload className="size-3.5" />
                    Upload
                </Button>
            </div>
        </div>
    );
}
