// pickViewerKind classifies an entry into the viewer kind that should
// render it. Driven primarily by the server-supplied mime type, with a
// filename-extension fallback for older agents that don't populate it.
//
// New viewer kinds (pdf, video, audio, markdown, …) plug in by adding
// their mime prefixes / extension lists below; the dispatcher in
// FileBrowser branches on the returned kind.

export type ViewerKind = "image" | "pdf" | "video" | "audio" | "markdown" | "text";

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

const VIDEO_EXT = new Set(["mp4", "m4v", "mov", "webm", "mkv", "avi"]);
const AUDIO_EXT = new Set(["mp3", "wav", "ogg", "oga", "flac", "m4a", "aac", "opus"]);
const MARKDOWN_EXT = new Set(["md", "markdown"]);

function extOf(name: string): string {
    const i = name.lastIndexOf(".");
    if (i < 0 || i === name.length - 1) return "";
    return name.slice(i + 1).toLowerCase();
}

export function pickViewerKind(mime: string | undefined, name: string): ViewerKind {
    const ext = extOf(name);

    // Markdown gets first dibs over the generic text/* match below so a
    // .md file with mime "text/plain" still routes to the rendered view.
    if (mime === "text/markdown" || MARKDOWN_EXT.has(ext)) return "markdown";

    if (mime === "application/pdf" || ext === "pdf") return "pdf";
    if (mime?.startsWith("image/") || IMAGE_EXT.has(ext)) return "image";
    if (mime?.startsWith("video/") || VIDEO_EXT.has(ext)) return "video";
    if (mime?.startsWith("audio/") || AUDIO_EXT.has(ext)) return "audio";

    return "text";
}
