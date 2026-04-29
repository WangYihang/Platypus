import type { SortingState } from "@tanstack/react-table";

import type { FileEntryDTO } from "../../../platform/App.web";
import { extKeyOf } from "./fileIcons";

// Sort ids accepted by the file explorer. "name" / "size" / "mode" /
// "modTimeUnix" mirror the table's accessor keys so a header click
// continues to round-trip through the same state. "type" is only
// surfaced via the toolbar's sort menu — it groups files by extension
// (with directories first), which has no column-equivalent.
export type FileSortId = "name" | "size" | "mode" | "modTimeUnix" | "type";

const collator = new Intl.Collator(undefined, {
    numeric: true,
    sensitivity: "base",
});

function compareName(a: FileEntryDTO, b: FileEntryDTO): number {
    return collator.compare(a.name, b.name);
}

function comparePrimary(a: FileEntryDTO, b: FileEntryDTO, id: FileSortId): number {
    switch (id) {
        case "size":
            return (a.size || 0) - (b.size || 0);
        case "mode":
            return (a.mode || 0) - (b.mode || 0);
        case "modTimeUnix":
            return (a.modTimeUnix || 0) - (b.modTimeUnix || 0);
        case "type": {
            // Directories first, symlinks next, then files grouped by
            // extension. Same-type entries tie-break on name so the
            // result is stable across renders.
            const rankA = a.isDir ? 0 : a.isSymlink ? 1 : 2;
            const rankB = b.isDir ? 0 : b.isSymlink ? 1 : 2;
            if (rankA !== rankB) return rankA - rankB;
            const extA = a.isDir || a.isSymlink ? "" : extKeyOf(a.name);
            const extB = b.isDir || b.isSymlink ? "" : extKeyOf(b.name);
            return collator.compare(extA, extB);
        }
        case "name":
        default:
            return compareName(a, b);
    }
}

// sortEntries returns a new array sorted by `sorting[0]` (we only ever
// honour a single sort key today). Falls back to name-asc when no key
// is provided. Always tie-breaks on name so the order is stable
// across rerenders, which keeps the grid view's react keys stable
// after a refetch.
export function sortEntries(
    entries: FileEntryDTO[],
    sorting: SortingState,
): FileEntryDTO[] {
    const head = sorting[0];
    const id = (head?.id as FileSortId) ?? "name";
    const desc = !!head?.desc;
    const out = entries.slice();
    out.sort((a, b) => {
        let cmp = comparePrimary(a, b, id);
        if (cmp === 0 && id !== "name") cmp = compareName(a, b);
        return desc ? -cmp : cmp;
    });
    return out;
}
