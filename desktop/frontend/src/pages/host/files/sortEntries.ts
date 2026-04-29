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

interface SortOptions {
    // When true, directories always rank above files (and symlinks
    // sit between the two) regardless of the user's active sort
    // key. The flip side is "merge mode": every entry is sorted on
    // the chosen key alone, which is what power users coming from a
    // bare `ls -la` workflow tend to want.
    foldersFirst?: boolean;
}

function typeRank(e: FileEntryDTO): number {
    if (e.isDir) return 0;
    if (e.isSymlink) return 1;
    return 2;
}

// sortEntries returns a new array sorted by `sorting[0]` (we only ever
// honour a single sort key today). Falls back to name-asc when no key
// is provided. Always tie-breaks on name so the order is stable
// across rerenders, which keeps the grid view's react keys stable
// after a refetch.
export function sortEntries(
    entries: FileEntryDTO[],
    sorting: SortingState,
    opts: SortOptions = {},
): FileEntryDTO[] {
    const head = sorting[0];
    const id = (head?.id as FileSortId) ?? "name";
    const desc = !!head?.desc;
    const foldersFirst = !!opts.foldersFirst;
    const out = entries.slice();
    out.sort((a, b) => {
        // The folders-first axis sits *outside* the user's chosen
        // direction so flipping to descending doesn't invert it —
        // operators expect "newest first" to mean newest *files*
        // among files, not the folders to drop to the bottom. The
        // "type" sort already groups by rank and is unaffected.
        if (foldersFirst && id !== "type") {
            const rankDiff = typeRank(a) - typeRank(b);
            if (rankDiff !== 0) return rankDiff;
        }
        let cmp = comparePrimary(a, b, id);
        if (cmp === 0 && id !== "name") cmp = compareName(a, b);
        return desc ? -cmp : cmp;
    });
    return out;
}
