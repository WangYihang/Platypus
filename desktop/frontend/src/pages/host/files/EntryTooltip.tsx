import { ReactNode } from "react";
import dayjs from "dayjs";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { FileEntryDTO } from "../../../platform/App.web";
import { humanize } from "../../../lib/format";
import { fromNow } from "../../../lib/time";
import { formatMode, formatModeOctal } from "./paths";
import { extKeyOf } from "./fileIcons";

interface Props {
    entry: FileEntryDTO;
    fullPath: string;
    children: ReactNode;
    // The list view's tooltip points right (sibling rows are
    // vertically adjacent so a top/bottom side would clip the
    // cursor). The grid view's points up so the tile below stays
    // legible. Default `right` is sane for the list path.
    side?: "top" | "right" | "bottom" | "left";
}

// EntryTooltip is the hover affordance shared by the list row and
// the grid tile. Renders the entry's full path, perms, size, and
// mtime in a small ledger so an operator doesn't have to click into
// the preview pane just to read metadata they remember.
//
// The hover delay (300 ms) is intentional: we don't want to flash
// the tooltip while an operator is sweep-clicking through a
// directory, only when they pause on a target.
export default function EntryTooltip({ entry, fullPath, children, side = "right" }: Props) {
    const ext = extKeyOf(entry.name);
    const kind = entry.isDir
        ? "Folder"
        : entry.isSymlink
            ? "Symbolic link"
            : ext
                ? `${ext.toUpperCase()} file`
                : "File";
    const mtime = entry.modTimeUnix
        ? dayjs(entry.modTimeUnix / 1_000_000)
        : null;
    return (
        <Tooltip delayDuration={300}>
            <TooltipTrigger asChild>{children}</TooltipTrigger>
            <TooltipContent
                side={side}
                align="start"
                className="max-w-xs space-y-0.5 p-2 text-[11px] font-normal leading-snug"
            >
                <div className="break-all font-mono">{fullPath}</div>
                <div className="text-[10px] opacity-80">{kind}</div>
                {!entry.isDir && (
                    <div>
                        <span className="opacity-60">Size:</span>{" "}
                        {humanize(entry.size)}
                    </div>
                )}
                <div>
                    <span className="opacity-60">Mode:</span>{" "}
                    <span className="font-mono">
                        {formatMode(entry.mode, entry.isDir, entry.isSymlink)} ·{" "}
                        {formatModeOctal(entry.mode)}
                    </span>
                </div>
                {mtime && (
                    <div>
                        <span className="opacity-60">Modified:</span>{" "}
                        {mtime.format("YYYY-MM-DD HH:mm:ss")}{" "}
                        <span className="opacity-60">({fromNow(mtime.toDate())})</span>
                    </div>
                )}
                {entry.isSymlink && entry.symlinkTarget && (
                    <div>
                        <span className="opacity-60">→</span>{" "}
                        <span className="break-all font-mono">{entry.symlinkTarget}</span>
                    </div>
                )}
                {entry.mime && !entry.isDir && (
                    <div>
                        <span className="opacity-60">MIME:</span>{" "}
                        <span className="font-mono">{entry.mime}</span>
                    </div>
                )}
            </TooltipContent>
        </Tooltip>
    );
}
