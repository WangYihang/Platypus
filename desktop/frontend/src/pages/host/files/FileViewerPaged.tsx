import { useCallback, useEffect, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { EditorState } from "@codemirror/state";
import { oneDark } from "@codemirror/theme-one-dark";
import { ChevronLeft, ChevronRight, Download, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ReadFile } from "../../../../wailsjs/go/app/App";
import { humanize } from "../../../lib/format";

// Read-only paged viewer for files that are too big to load into a
// CodeMirror editor in full. Each page is 64 KiB — small enough to
// render instantly, big enough to spot patterns in logs. Users who
// need to *edit* large files are instructed to Download-Edit-Upload.

const PAGE_SIZE = 64 * 1024;

interface Props {
    projectID: string;
    sessionHash: string;
    path: string;
    size: number;
    onDownload?: () => void;
}

function bytesToText(raw: unknown): string {
    if (raw instanceof Uint8Array) {
        return safeDecode(raw);
    }
    if (Array.isArray(raw)) {
        return safeDecode(new Uint8Array(raw as number[]));
    }
    return "";
}

function safeDecode(bytes: Uint8Array): string {
    try {
        return new TextDecoder("utf-8").decode(bytes);
    } catch {
        return new TextDecoder("latin1").decode(bytes);
    }
}

export default function FileViewerPaged({ projectID, sessionHash, path, size, onDownload }: Props) {
    const [offset, setOffset] = useState(0);
    const [content, setContent] = useState("");
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [gotoInput, setGotoInput] = useState("0");

    const loadPage = useCallback(
        async (at: number) => {
            setLoading(true);
            setError(null);
            try {
                const clamped = Math.max(0, Math.min(at, Math.max(0, size - 1)));
                const want = Math.min(PAGE_SIZE, Math.max(0, size - clamped));
                const raw = await ReadFile(projectID, sessionHash, path, clamped, want);
                setContent(bytesToText(raw));
                setOffset(clamped);
                setGotoInput(String(clamped));
            } catch (err) {
                setError(String(err instanceof Error ? err.message : err));
            } finally {
                setLoading(false);
            }
        },
        [projectID, sessionHash, path, size],
    );

    useEffect(() => {
        loadPage(0);
    }, [loadPage]);

    const page = Math.floor(offset / PAGE_SIZE) + 1;
    const totalPages = Math.max(1, Math.ceil(size / PAGE_SIZE));

    return (
        <div className="flex h-full flex-col">
            <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                <div className="truncate font-mono">{path}</div>
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    read-only — {humanize(size)} is too large to edit inline; download to edit
                </div>
                {onDownload && (
                    <Button type="button" size="sm" variant="outline" onClick={onDownload}>
                        <Download className="size-3.5" />
                        Download
                    </Button>
                )}
            </div>
            <div className="flex items-center gap-2 border-b px-3 py-2 text-sm">
                <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={offset === 0 || loading}
                    onClick={() => loadPage(offset - PAGE_SIZE)}
                >
                    <ChevronLeft className="size-3.5" />
                    Prev
                </Button>
                <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={offset + PAGE_SIZE >= size || loading}
                    onClick={() => loadPage(offset + PAGE_SIZE)}
                >
                    Next
                    <ChevronRight className="size-3.5" />
                </Button>
                <span className="text-muted-foreground">
                    page {page} / {totalPages} (offset {offset})
                </span>
                <form
                    className="ml-auto flex items-center gap-2"
                    onSubmit={(e) => {
                        e.preventDefault();
                        const n = parseInt(gotoInput, 10);
                        if (Number.isFinite(n)) loadPage(n);
                    }}
                >
                    <span className="text-muted-foreground">go to offset:</span>
                    <Input
                        value={gotoInput}
                        onChange={(e) => setGotoInput(e.target.value)}
                        className="h-8 w-28 font-mono"
                    />
                </form>
            </div>
            <div className="flex-1 overflow-hidden">
                {loading ? (
                    <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                        <Loader2 className="size-4 animate-spin" />
                        Reading bytes {offset}–{Math.min(offset + PAGE_SIZE, size)}…
                    </div>
                ) : error ? (
                    <div className="flex h-full items-center justify-center text-sm text-red-500">
                        {error}
                    </div>
                ) : (
                    <CodeMirror
                        value={content}
                        height="100%"
                        theme={oneDark}
                        extensions={[EditorState.readOnly.of(true)]}
                        basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: false }}
                    />
                )}
            </div>
        </div>
    );
}
