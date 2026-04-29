import { describe, expect, it } from "vitest";

import { extKeyOf, isHiddenEntry, pickFileIcon } from "./fileIcons";
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

describe("extKeyOf", () => {
    it("returns the lowercased suffix of the filename", () => {
        expect(extKeyOf("README.md")).toBe("md");
        expect(extKeyOf("photo.JPG")).toBe("jpg");
    });

    it("recognises compound archive extensions", () => {
        expect(extKeyOf("dump.tar.gz")).toBe("tar.gz");
        expect(extKeyOf("dump.tar.xz")).toBe("tar.xz");
    });

    it("returns an empty string for dotfiles and bare names", () => {
        expect(extKeyOf(".bashrc")).toBe("");
        expect(extKeyOf("Makefile")).toBe("");
        expect(extKeyOf("trailing.")).toBe("");
    });
});

describe("pickFileIcon", () => {
    it("routes documents to their tinted icon", () => {
        const pdf = pickFileIcon(file("manual.pdf"));
        expect(pdf.color).toBe("text-red-500");
        const docx = pickFileIcon(file("contract.docx"));
        expect(docx.color).toBe("text-blue-500");
    });

    it("uses the dir / symlink fallbacks regardless of extension", () => {
        const dir = pickFileIcon(file("photos.png", { isDir: true }));
        expect(dir.color).toBe("text-amber-500");
        const link = pickFileIcon(file("link.png", { isSymlink: true }));
        expect(link.color).toBe("text-sky-500");
    });

    it("falls back to the muted file icon when the extension is unknown", () => {
        const fallback = pickFileIcon(file("blob.unknownext"));
        expect(fallback.color).toBe("text-muted-foreground");
    });
});

describe("isHiddenEntry", () => {
    it("treats dotfiles as hidden", () => {
        expect(isHiddenEntry(file(".bashrc"))).toBe(true);
        expect(isHiddenEntry(file(".git", { isDir: true }))).toBe(true);
    });

    it("does not treat regular files as hidden", () => {
        expect(isHiddenEntry(file("README.md"))).toBe(false);
        expect(isHiddenEntry(file("notes"))).toBe(false);
    });
});
