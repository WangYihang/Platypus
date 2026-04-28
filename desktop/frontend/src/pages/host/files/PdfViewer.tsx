import { useEffect, useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ReadFile } from "@wails/go/app/App";
import { humanize } from "../../../lib/format";
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

function bytesFromWailsRead(raw: unknown): Uint8Array {
    if (raw instanceof Uint8Array) return raw;
    if (Array.isArray(raw)) return new Uint8Array(raw as number[]);
    throw new Error(`unexpected ReadFile shape: ${typeof raw}`);
}

export default function PdfViewer({ projectID, sessionHash, path, size }: Props) {
    // Blob URL, not raw bytes. Passing { data: Uint8Array } made pdfjs
    // postMessage the underlying ArrayBuffer to its worker as a
    // transferable, which detached the buffer; the next render then
    // crashed with "ArrayBuffer is already detached". A Blob URL is
    // stable under React re-renders, lets pdfjs fetch the document
    // however it likes internally, and revokes cleanly on unmount.
    const [url, setUrl] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [numPages, setNumPages] = useState(0);
    const [pageNumber, setPageNumber] = useState(1);

    useEffect(() => {
        let cancelled = false;
        let createdURL: string | null = null;
        setUrl(null);
        setError(null);
        setPageNumber(1);
        setNumPages(0);
        (async () => {
            try {
                const raw = await ReadFile(projectID, sessionHash, path, 0, 0);
                if (cancelled) return;
                const bytes = bytesFromWailsRead(raw);
                const blob = new Blob([bytes as BlobPart], { type: "application/pdf" });
                createdURL = URL.createObjectURL(blob);
                setUrl(createdURL);
            } catch (err) {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : String(err));
            }
        })();
        return () => {
            cancelled = true;
            if (createdURL) URL.revokeObjectURL(createdURL);
        };
    }, [projectID, sessionHash, path]);

    if (error) {
        return (
            <div className="flex h-full items-center justify-center px-4 text-center text-sm text-red-500">
                {error}
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
                            <span className="text-xs text-muted-foreground">
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
                    onLoadError={(err) => setError(err instanceof Error ? err.message : String(err))}
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
