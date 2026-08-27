import { Loader2 } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

import { humanize } from "../../../lib/format";
import { useRemoteFileText } from "./remoteFile";

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

export default function MarkdownViewer({ projectID, sessionHash, path, size }: Props) {
    const tooLarge = size > MAX_INLINE_MARKDOWN_BYTES;

    const { text, error: loadError } = useRemoteFileText({
        projectID,
        sessionHash,
        path,
        enabled: !tooLarge,
    });
    const error = loadError
        ? loadError instanceof Error
            ? loadError.message
            : String(loadError)
        : null;

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
