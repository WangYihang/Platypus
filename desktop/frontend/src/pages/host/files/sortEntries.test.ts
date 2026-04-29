import { describe, expect, it } from "vitest";

import { sortEntries } from "./sortEntries";
import type { FileEntryDTO } from "../../../platform/App.web";

function file(name: string, extra: Partial<FileEntryDTO> = {}): FileEntryDTO {
    return {
        name,
        size: 0,
        mode: 0o644,
        modTimeUnix: 0,
        isDir: false,
        isSymlink: false,
        ...extra,
    };
}

describe("sortEntries", () => {
    it("sorts by name ascending by default", () => {
        const out = sortEntries(
            [file("c.txt"), file("a.txt"), file("b.txt")],
            [],
        );
        expect(out.map((e) => e.name)).toEqual(["a.txt", "b.txt", "c.txt"]);
    });

    it("uses a numeric collator so 'log9' comes before 'log10'", () => {
        const out = sortEntries(
            [file("log10.txt"), file("log2.txt"), file("log9.txt")],
            [{ id: "name", desc: false }],
        );
        expect(out.map((e) => e.name)).toEqual(["log2.txt", "log9.txt", "log10.txt"]);
    });

    it("flips order when desc is set", () => {
        const out = sortEntries(
            [file("a.txt"), file("b.txt")],
            [{ id: "name", desc: true }],
        );
        expect(out.map((e) => e.name)).toEqual(["b.txt", "a.txt"]);
    });

    it("sorts by size and breaks ties on name", () => {
        const out = sortEntries(
            [
                file("big.bin", { size: 1000 }),
                file("z.txt", { size: 10 }),
                file("a.txt", { size: 10 }),
            ],
            [{ id: "size", desc: false }],
        );
        expect(out.map((e) => e.name)).toEqual(["a.txt", "z.txt", "big.bin"]);
    });

    it("groups by type with directories first", () => {
        const out = sortEntries(
            [
                file("notes.txt"),
                file("src", { isDir: true }),
                file("photo.jpg"),
                file("link", { isSymlink: true }),
            ],
            [{ id: "type", desc: false }],
        );
        expect(out.map((e) => e.name)).toEqual(["src", "link", "photo.jpg", "notes.txt"]);
    });

    it("with foldersFirst, dirs always come before files even when sort key reverses", () => {
        const entries = [
            file("a.txt", { modTimeUnix: 100 }),
            file("zebra-dir", { isDir: true, modTimeUnix: 1 }),
            file("b.txt", { modTimeUnix: 200 }),
        ];
        const out = sortEntries(
            entries,
            [{ id: "modTimeUnix", desc: true }],
            { foldersFirst: true },
        );
        // The dir lands first regardless of its mtime; the files
        // then sort newest-first among themselves.
        expect(out.map((e) => e.name)).toEqual(["zebra-dir", "b.txt", "a.txt"]);
    });

    it("foldersFirst is bypassed when sort key is 'type'", () => {
        // The "type" key already groups by rank, so layering
        // foldersFirst on top would be a redundant constraint that
        // could surprise users who flip "Files first" via a desc=true.
        const out = sortEntries(
            [
                file("src", { isDir: true }),
                file("a.txt"),
            ],
            [{ id: "type", desc: true }],
            { foldersFirst: true },
        );
        expect(out.map((e) => e.name)).toEqual(["a.txt", "src"]);
    });
});
