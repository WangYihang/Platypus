import dayjs from "dayjs";

import type { FileEntryDTO } from "../../../platform/App.web";
import { humanize } from "../../../lib/format";
import { fromNow } from "../../../lib/time";
import { extKeyOf } from "./fileIcons";
import { formatMode, formatModeOctal } from "./paths";

// entryTitleText renders the same metadata bundle as <EntryTooltip>
// into a multi-line plain-text string suitable for an HTMLElement's
// `title` attribute. Used by the grid view, where wrapping a Radix
// Tooltip around the per-tile context-menu trigger doesn't compose
// cleanly with `asChild`. The list view uses the richer Radix
// tooltip directly because its cell renders a plain <div>.
//
// Tradeoff: native browser tooltips don't honour styling and only
// some browsers respect newlines (Chrome and Firefox do; Safari
// flattens them to spaces). We accept that — the alternative is
// re-architecting the per-row context menu which would touch a lot
// of unrelated code.
export function entryTitleText(entry: FileEntryDTO, fullPath: string): string {
    const lines: string[] = [];
    lines.push(fullPath);
    const ext = extKeyOf(entry.name);
    const kind = entry.isDir
        ? "Folder"
        : entry.isSymlink
            ? "Symbolic link"
            : ext
                ? `${ext.toUpperCase()} file`
                : "File";
    lines.push(kind);
    if (!entry.isDir) {
        lines.push(`Size: ${humanize(entry.size)}`);
    }
    lines.push(
        `Mode: ${formatMode(entry.mode, entry.isDir, entry.isSymlink)} (${formatModeOctal(entry.mode)})`,
    );
    if (entry.modTimeUnix) {
        const m = dayjs(entry.modTimeUnix / 1_000_000);
        lines.push(
            `Modified: ${m.format("YYYY-MM-DD HH:mm:ss")} (${fromNow(m.toDate())})`,
        );
    }
    if (entry.isSymlink && entry.symlinkTarget) {
        lines.push(`→ ${entry.symlinkTarget}`);
    }
    if (entry.mime && !entry.isDir) {
        lines.push(`MIME: ${entry.mime}`);
    }
    return lines.join("\n");
}
