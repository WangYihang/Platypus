import { useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { humanize } from "../../../lib/format";
import { useRemotePreviewURL } from "./remoteFile";
// Absolute import (rather than "./pdfWorkerSrc") so vitest's alias
// table can swap in a stub — vitest aliases only match against the
// import specifier as written, before relative-path resolution.
import workerSrc from "@/pages/host/files/pdfWorkerSrc";

// react-pdf needs an explicit worker URL. The URL comes from a
// dedicated module so Vite can rewrite the `?url` import to the
// emitted asset path; doing it inline here would confuse vitest's
// import-analysis. Without a real URL pdfjs falls back to the
// in-thread "fake worker" path and fetches a 404, which is what
// produced "Setting up fake worker failed" in dev.
pdfjs.GlobalWorkerOptions.workerSrc = workerSrc;

interface Props {
    projectID: string;
    sessionHash: string;
    path: string;
    size: number;
}

export default function PdfViewer({ projectID, sessionHash, path, size }: Props) {
    // Web mode hands pdf.js a preview URL so it can Range-fetch pages
    // lazily; desktop reads the whole file and wraps it in a blob URL,
    // because pdf.js cannot Range-fetch over Wails IPC. Both live in
    // useRemotePreviewURL now — MediaViewer needed the same choice.
    //
    // A blob URL specifically, not raw bytes: passing { data:
    // Uint8Array } made pdfjs postMessage the underlying ArrayBuffer to
    // its worker as a transferable, which detached the buffer and
    // crashed the next render with "ArrayBuffer is already detached".
    const { url, error: loadError } = useRemotePreviewURL({
        projectID,
        sessionHash,
        path,
        mime: "application/pdf",
    });

    if (loadError) {
        return (
            <div className="flex h-full items-center justify-center px-4 text-center text-sm text-red-500">
                {loadError instanceof Error ? loadError.message : String(loadError)}
            </div>
        );
    }

    if (!url) {
        return (
            <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                Loading {humanize(size)}…
            </div>
        );
    }

    // Keyed on the url so page position and page count are discarded
    // when the document changes. They belong to one document, so the
    // component that owns them is identified by that document —
    // rather than an effect that reaches in and resets them whenever
    // the path prop moves.
    return <PdfDocument key={url} url={url} path={path} />;
}

function PdfDocument({ url, path }: { url: string; path: string }) {
    const [numPages, setNumPages] = useState(0);
    const [pageNumber, setPageNumber] = useState(1);
    const [renderError, setRenderError] = useState<string | null>(null);

    if (renderError) {
        return (
            <div className="flex h-full items-center justify-center px-4 text-center text-sm text-red-500">
                {renderError}
            </div>
        );
    }

    return (
        <div className="flex h-full flex-col">
            <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                <div className="truncate font-mono">{path}</div>
                <div className="flex items-center gap-2">
                    {numPages > 0 && (
                        <>
                            <Button
                                type="button"
                                size="sm"
                                variant="ghost"
                                aria-label="Previous page"
                                disabled={pageNumber <= 1}
                                onClick={() => setPageNumber((p) => Math.max(1, p - 1))}
                            >
                                <ChevronLeft className="size-3.5" />
                                Prev
                            </Button>
                            <span className="text-xs textate-muted-foreground">
                                Page {pageNumber} of {numPages}
                            </span>
                            <Button
                                type="button"
                                size="sm"
                                variant="ghost"
                                aria-label="Next page"
                                disabled={pageNumber >= numPages}
                                onClick={() =>
                                    setPageNumber((p) => Math.min(numPages, p + 1))
                                }
                            >
                                Next
                                <ChevronRight className="size-3.5" />
                            </Button>
                        </>
                    )}
                </div>
            </div>
            <div className="flex flex-1 items-center justify-center overflow-auto bg-[color:var(--muted)] p-4">
                <Document
                    file={url}
                    onLoadSuccess={(info) => setNumPages(info.numPages)}
                    onLoadError={(err) =>
                        setRenderError(err instanceof Error ? err.message : String(err))
                    }
                    loading={
                        <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Loader2 className="size-4 animate-spin" />
                            Rendering…
                        </div>
                    }
                >
                    <Page pageNumber={pageNumber} />
                </Document>
            </div>
        </div>
    );
}
