import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

import { ReadFile } from "@wails/go/app/App";
import { humanize } from "../../../lib/format";

// 4 MiB is the cap the viewer is willing to feed react-markdown +
// remark-gfm. Above this point the parser walk and the rendered
// DOM both choke; the user should download and view in a real
// editor instead.
const MAX_INLINE_MARKDOWN_BYTES = 4 * 1024 * 1024;

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

function decodeText(bytes: Uint8Array): string {
    try {
        return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    } catch {
        return new TextDecoder("latin1").decode(bytes);
    }
}

export default function MarkdownViewer({ projectID, sessionHash, path, size }: Props) {
    const [text, setText] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    const tooLarge = size > MAX_INLINE_MARKDOWN_BYTES;

    useEffect(() => {
        if (tooLarge) return;
        let cancelled = false;
        setText(null);
        setError(null);
        (async () => {
            try {
                const raw = await ReadFile(projectID, sessionHash, path, 0, 0);
                if (cancelled) return;
                setText(decodeText(bytesFromWailsRead(raw)));
            } catch (err) {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : String(err));
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [projectID, sessionHash, path, tooLarge]);

    if (tooLarge) {
        return (
            <div className="flex h-full flex-col">
                <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                    <div className="truncate font-mono">{path}</div>
                    <div className="text-xs text-muted-foreground">markdown · {humanize(size)}</div>
                </div>
                <div className="flex flex-1 items-center justify-center px-4 text-center text-sm text-muted-foreground">
                    File is {humanize(size)} — too large to preview inline.
                    Use the toolbar's Download action to view it in a real editor.
                </div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex h-full items-center justify-center px-4 text-center text-sm text-red-500">
                {error}
            </div>
        );
    }

    if (text === null) {
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
                <div className="text-xs text-muted-foreground">markdown · {humanize(size)}</div>
            </div>
            <div className="flex-1 overflow-auto px-6 py-4">
                <article className="prose prose-sm max-w-none dark:prose-invert">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{text}</ReactMarkdown>
                </article>
            </div>
        </div>
    );
}
