import { useEffect, useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ReadFile } from "@wails/go/app/App";
import { humanize } from "../../../lib/format";

// react-pdf needs an explicit worker URL. Resolved relative to this
// module so Vite bundles the worker alongside the rest of the assets;
// without it pdfjs falls back to fake-worker mode which is single-
// threaded and stalls on heavy docs.
pdfjs.GlobalWorkerOptions.workerSrc = new URL(
    "pdfjs-dist/build/pdf.worker.min.mjs",
    import.meta.url,
).toString();

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
    const [data, setData] = useState<Uint8Array | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [numPages, setNumPages] = useState(0);
    const [pageNumber, setPageNumber] = useState(1);

    useEffect(() => {
        let cancelled = false;
        setData(null);
        setError(null);
        setPageNumber(1);
        setNumPages(0);
        (async () => {
            try {
                const raw = await ReadFile(projectID, sessionHash, path, 0, 0);
                if (cancelled) return;
                setData(bytesFromWailsRead(raw));
            } catch (err) {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : String(err));
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [projectID, sessionHash, path]);

    if (error) {
        return (
            <div className="flex h-full items-center justify-center px-4 text-center text-sm text-red-500">
                {error}
            </div>
        );
    }

    if (!data) {
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
                    file={{ data }}
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
