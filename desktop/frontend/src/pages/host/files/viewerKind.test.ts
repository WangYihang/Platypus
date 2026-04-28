import { describe, expect, it } from "vitest";

import { pickViewerKind } from "./viewerKind";

describe("pickViewerKind", () => {
    it("returns image for image mime types", () => {
        expect(pickViewerKind("image/png", "logo.png")).toBe("image");
        expect(pickViewerKind("image/jpeg", "x.jpg")).toBe("image");
        expect(pickViewerKind("image/svg+xml", "i.svg")).toBe("image");
        expect(pickViewerKind("image/webp", "y.webp")).toBe("image");
    });

    it("returns pdf for application/pdf and .pdf names", () => {
        expect(pickViewerKind("application/pdf", "spec.pdf")).toBe("pdf");
        // mime missing — extension fallback still picks pdf.
        expect(pickViewerKind(undefined, "spec.PDF")).toBe("pdf");
        expect(pickViewerKind("application/octet-stream", "x.pdf")).toBe("pdf");
    });

    it("returns image when mime is missing but extension is an image", () => {
        // Older agents may not populate mime; fall back to extension.
        expect(pickViewerKind(undefined, "vacation.PNG")).toBe("image");
        expect(pickViewerKind("", "x.jpeg")).toBe("image");
        expect(pickViewerKind("application/octet-stream", "shot.gif")).toBe("image");
    });

    it("returns video for video mime types", () => {
        expect(pickViewerKind("video/mp4", "clip.mp4")).toBe("video");
        expect(pickViewerKind("video/webm", "x.webm")).toBe("video");
        expect(pickViewerKind(undefined, "movie.mkv")).toBe("video");
        expect(pickViewerKind("application/octet-stream", "x.MOV")).toBe("video");
    });

    it("returns audio for audio mime types", () => {
        expect(pickViewerKind("audio/mpeg", "song.mp3")).toBe("audio");
        expect(pickViewerKind("audio/wav", "voice.wav")).toBe("audio");
        expect(pickViewerKind(undefined, "tone.OGG")).toBe("audio");
        expect(pickViewerKind(undefined, "x.flac")).toBe("audio");
    });

    it("returns markdown for *.md and text/markdown", () => {
        // Markdown has its own viewer (rendered preview); plain
        // text/* falls through to the text editor as before.
        expect(pickViewerKind("text/markdown", "README.md")).toBe("markdown");
        expect(pickViewerKind(undefined, "README.MD")).toBe("markdown");
        expect(pickViewerKind("text/plain", "spec.markdown")).toBe("markdown");
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
