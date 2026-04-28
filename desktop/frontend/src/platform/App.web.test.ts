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
});
