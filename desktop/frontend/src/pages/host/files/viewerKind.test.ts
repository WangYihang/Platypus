import { describe, expect, it } from "vitest";

import { pickViewerKind } from "./viewerKind";

describe("pickViewerKind", () => {
    it("returns image for image mime types", () => {
        expect(pickViewerKind("image/png", "logo.png")).toBe("image");
        expect(pickViewerKind("image/jpeg", "x.jpg")).toBe("image");
        expect(pickViewerKind("image/svg+xml", "i.svg")).toBe("image");
        expect(pickViewerKind("image/webp", "y.webp")).toBe("image");
    });

    it("returns image when mime is missing but extension is an image", () => {
        // Older agents may not populate mime; fall back to extension.
        expect(pickViewerKind(undefined, "vacation.PNG")).toBe("image");
        expect(pickViewerKind("", "x.jpeg")).toBe("image");
        expect(pickViewerKind("application/octet-stream", "shot.gif")).toBe("image");
    });

    it("returns text for text-y mimes and editable code", () => {
        expect(pickViewerKind("text/plain", "x.txt")).toBe("text");
        expect(pickViewerKind("text/typescript", "App.tsx")).toBe("text");
        expect(pickViewerKind("application/json", "d.json")).toBe("text");
        expect(pickViewerKind("application/yaml", "c.yaml")).toBe("text");
    });

    it("returns text as a safe fallback for unknown types", () => {
        // Editor handles binary gracefully — unknown still maps to text
        // so users can at least try opening it. Future viewers can claim
        // their kinds explicitly.
        expect(pickViewerKind(undefined, "Makefile")).toBe("text");
        expect(pickViewerKind("application/octet-stream", "data.bin")).toBe("text");
    });
});
