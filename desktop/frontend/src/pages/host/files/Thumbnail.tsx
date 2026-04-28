import { useEffect, useRef, useState } from "react";
import { ImageIcon } from "lucide-react";

import { ReadFile } from "@wails/go/app/App";

interface Props {
    projectID: string;
    sessionHash: string;
    path: string;
    mime?: string;
}

function bytesFromWailsRead(raw: unknown): Uint8Array | null {
    if (raw instanceof Uint8Array) return raw;
    if (Array.isArray(raw)) return new Uint8Array(raw as number[]);
    return null;
}

// Thumbnail lazy-loads an image preview only after the tile scrolls
// into the viewport. Uses IntersectionObserver so a 500-image
// directory doesn't fan out 500 ReadFile RPCs at mount time. Fetch
// is one-shot per (path, mime); a load failure leaves the placeholder
// in place rather than escalating into a visible error.
export default function Thumbnail({ projectID, sessionHash, path, mime }: Props) {
    const ref = useRef<HTMLDivElement | null>(null);
    const [url, setUrl] = useState<string | null>(null);
    const [failed, setFailed] = useState(false);

    useEffect(() => {
        const node = ref.current;
        if (!node) return;
        let cancelled = false;
        let createdURL: string | null = null;

        const load = () => {
            (async () => {
                try {
                    const raw = await ReadFile(projectID, sessionHash, path, 0, 0);
                    if (cancelled) return;
                    const bytes = bytesFromWailsRead(raw);
                    if (!bytes) {
                        setFailed(true);
                        return;
                    }
                    const blob = new Blob([bytes as BlobPart], { type: mime || "image/*" });
                    createdURL = URL.createObjectURL(blob);
                    setUrl(createdURL);
                } catch {
                    if (!cancelled) setFailed(true);
                }
            })();
        };

        const obs = new IntersectionObserver(
            (entries) => {
                for (const e of entries) {
                    if (e.isIntersecting) {
                        load();
                        obs.disconnect();
                        break;
                    }
                }
            },
            { rootMargin: "200px" },
        );
        obs.observe(node);

        return () => {
            cancelled = true;
            obs.disconnect();
            if (createdURL) URL.revokeObjectURL(createdURL);
        };
    }, [projectID, sessionHash, path, mime]);

    return (
        <div ref={ref} className="flex size-full items-center justify-center">
            {url ? (
                <img
                    src={url}
                    alt=""
                    className="size-full object-cover"
                />
            ) : failed ? (
                <ImageIcon className="size-8 text-muted-foreground" />
            ) : (
                <ImageIcon className="size-8 text-muted-foreground/40" />
            )}
        </div>
    );
}
