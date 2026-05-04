import { describe, expect, it } from "vitest";

import { adaptEntry } from "./App.web";

const GO_MODE_DIR = 1 << 31;
const GO_MODE_SYMLINK = 1 << 27;

describe("adaptEntry", () => {
    it("forwards backend-provided mime onto the DTO", () => {
        const got = adaptEntry({
            name: "logo.png",
            mode: 0o644,
            size: 4096,
            mtime_unix_nano: 1700000000000000000,
            mime: "image/png",
        });
        expect(got.mime).toBe("image/png");
        expect(got.name).toBe("logo.png");
        expect(got.size).toBe(4096);
        expect(got.isDir).toBe(false);
        expect(got.isSymlink).toBe(false);
    });

    it("leaves mime undefined when backend omits it", () => {
        // Older agents / not-yet-upgraded servers won't include the field.
        // The DTO should keep working — mime is optional, not required.
        const got = adaptEntry({
            name: "blob",
            mode: 0o644,
            size: 0,
            mtime_unix_nano: 0,
        });
        expect(got.mime).toBeUndefined();
    });

    it("preserves the inode/* mime for directories", () => {
        const got = adaptEntry({
            name: "src",
            mode: GO_MODE_DIR | 0o755,
            size: 0,
            mtime_unix_nano: 0,
            mime: "inode/directory",
        });
        expect(got.isDir).toBe(true);
        expect(got.mime).toBe("inode/directory");
    });

    it("preserves the inode/symlink mime for symlinks", () => {
        const got = adaptEntry({
            name: "link",
            mode: GO_MODE_SYMLINK | 0o777,
            size: 0,
            mtime_unix_nano: 0,
            symlink_target: "/etc/hosts",
            mime: "inode/symlink",
        });
        expect(got.isSymlink).toBe(true);
        expect(got.symlinkTarget).toBe("/etc/hosts");
        expect(got.mime).toBe("inode/symlink");
    });

    // Post-Phase-B regression: the wasm sys-files-read plugin (was
    // sys-listdir before the merge) emits raw POSIX mode bits
    // (S_IFDIR = 0o040000, bit 14) instead of Go's os.FileMode
    // (ModeDir = 1 << 31). Without this branch in adaptEntry, every
    // directory in a wasm-served listing renders as a regular file
    // — folder icons disappear and double-clicking a directory
    // opens the file editor instead of navigating.
    it("recognises POSIX S_IFDIR (0o040000) as a directory", () => {
        const got = adaptEntry({
            name: "home",
            mode: 0o040755, // POSIX dir mode emitted by sys-files-read
            size: 0,
            mtime_unix_nano: 0,
        });
        expect(got.isDir).toBe(true);
        expect(got.isSymlink).toBe(false);
    });

    it("recognises POSIX S_IFLNK (0o120000) as a symlink", () => {
        const got = adaptEntry({
            name: "link",
            mode: 0o120777,
            size: 0,
            mtime_unix_nano: 0,
            symlink_target: "/usr/bin/python3",
        });
        expect(got.isSymlink).toBe(true);
        expect(got.isDir).toBe(false);
    });

    it("treats POSIX regular files (0o100644) as non-dir / non-symlink", () => {
        // S_IFREG = 0o100000 — must NOT be misclassified as dir or
        // symlink. A miscategorisation here would either hide the
        // file or send the file's content into the symlink toast.
        const got = adaptEntry({
            name: "flag.txt",
            mode: 0o100644,
            size: 11,
            mtime_unix_nano: 0,
        });
        expect(got.isDir).toBe(false);
        expect(got.isSymlink).toBe(false);
    });
});
