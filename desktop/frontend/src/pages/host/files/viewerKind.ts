// pickViewerKind classifies an entry into the viewer kind that should
// render it. Driven primarily by the server-supplied mime type, with a
// filename-extension fallback for older agents that don't populate it.
//
// New viewer kinds (pdf, video, audio, markdown, …) plug in by adding
// their mime prefixes / extension lists below; the dispatcher in
// FileBrowser branches on the returned kind.

export type ViewerKind = "image" | "pdf" | "text";

const IMAGE_EXT = new Set([
    "png",
    "jpg",
    "jpeg",
    "gif",
    "webp",
    "svg",
    "bmp",
    "ico",
    "tif",
    "tiff",
    "avif",
    "heic",
]);

function extOf(name: string): string {
    const i = name.lastIndexOf(".");
    if (i < 0 || i === name.length - 1) return "";
    return name.slice(i + 1).toLowerCase();
}

export function pickViewerKind(mime: string | undefined, name: string): ViewerKind {
    if (mime === "application/pdf") return "pdf";
    if (mime && mime.startsWith("image/")) return "image";

    // Server didn't classify (older agent or unknown ext on the server) —
    // fall back to extension sniffing for the cases viewers care about.
    const ext = extOf(name);
    if (ext === "pdf") return "pdf";
    if (IMAGE_EXT.has(ext)) return "image";

    return "text";
}
