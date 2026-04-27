import { describe, expect, it } from "vitest";

import {
    ARCHIVE_FORMATS,
    archiveExtension,
    archiveLabel,
    archiveMimeType,
    suggestedArchiveFilename,
} from "./archive";

// archive.ts is the single source of truth for "what compression
// formats does the folder-download modal offer, and how does each one
// map onto a file extension / MIME type / display label?". The dialog
// reads this, the backend reads this, and the test pinning here makes
// sure they don't drift.

describe("archive format helpers", () => {
    it("exposes tar, tar.gz, and zip in the offered set", () => {
        expect(ARCHIVE_FORMATS).toEqual(["tar.gz", "tar", "zip"]);
    });

    it("maps every format to a file extension", () => {
        expect(archiveExtension("tar")).toBe(".tar");
        expect(archiveExtension("tar.gz")).toBe(".tar.gz");
        expect(archiveExtension("zip")).toBe(".zip");
    });

    it("maps every format to a MIME type", () => {
        expect(archiveMimeType("tar")).toBe("application/x-tar");
        expect(archiveMimeType("tar.gz")).toBe("application/gzip");
        expect(archiveMimeType("zip")).toBe("application/zip");
    });

    it("returns a human-friendly label for each format", () => {
        expect(archiveLabel("tar")).toMatch(/tar/i);
        expect(archiveLabel("tar.gz")).toMatch(/tar\.gz|tarball|gzip/i);
        expect(archiveLabel("zip")).toMatch(/zip/i);
    });

    it("builds a suggested filename from a folder name + format", () => {
        expect(suggestedArchiveFilename("nginx", "tar.gz")).toBe("nginx.tar.gz");
        expect(suggestedArchiveFilename("nginx", "tar")).toBe("nginx.tar");
        expect(suggestedArchiveFilename("nginx", "zip")).toBe("nginx.zip");
    });

    it("falls back to 'archive' when the folder name is empty/'/' style", () => {
        // The remote path could be "/" — its base() is "" or "/"; in
        // either case "archive.tar.gz" is sensible.
        expect(suggestedArchiveFilename("", "tar.gz")).toBe("archive.tar.gz");
        expect(suggestedArchiveFilename("/", "zip")).toBe("archive.zip");
    });

    it("composes a filename from multiple selections", () => {
        // When more than one entry is selected, the filename should
        // not pretend to be one folder.
        expect(suggestedArchiveFilename(["a", "b"], "tar.gz")).toBe(
            "selection.tar.gz",
        );
    });
});
