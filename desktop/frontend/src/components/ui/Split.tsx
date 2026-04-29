import {
    Children,
    ReactNode,
    useCallback,
    useEffect,
    useRef,
    useState,
} from "react";

import { cn } from "@/lib/cn";

const STORAGE_PREFIX = "platypus.split.";

// `<Split>` is a two-pane layout with a draggable seam in between.
// The two children render as flex items that share the cross axis;
// the first child gets `pct%` of the main axis, the second gets the
// rest. Drag the seam → `pct` updates → optional persistence to
// localStorage.
//
// Replaces the previous `react-resizable-panels` + shadcn-wrapper
// combo. The library bought us percentage layouts and an imperative
// resize() API, but the only real use-site (FileBrowser's preview
// split) needed neither — just a percent-stored seam between two
// flex children. The wrapper's arbitrary-variant CSS picked the
// descendant combinator (`[data-panel-group-direction=vertical]_&]`)
// for its vertical-orientation overrides, which silently matched any
// vertical-Group ancestor and devoured the inner row's width when
// two Groups were nested. Custom-rolling avoids that whole class of
// surprise.

export interface SplitProps {
    /** Layout direction. `"row"` → vertical seam, panes side-by-side
     *  (default). `"column"` → horizontal seam, panes stacked. */
    direction?: "row" | "column";
    /** Percentage of the main axis given to the first pane (0..100).
     *  Default 50. Ignored when a persisted value is read from
     *  `storageKey`. */
    defaultPercent?: number;
    /** Lower bound for the first pane's percent (0..100). The seam
     *  drag clamps to this. Default 10. */
    minPercent?: number;
    /** Upper bound for the first pane's percent (0..100). Default 90. */
    maxPercent?: number;
    /** Persist the current percent under `platypus.split.<storageKey>`
     *  so the layout survives reloads. Omit to keep it per-mount. */
    storageKey?: string;
    /** Class on the root flex container. Almost always
     *  `min-h-0 flex-1` or similar so the parent's flex sizing
     *  reaches the panes. */
    className?: string;
    /** Exactly two children: `[firstPane, secondPane]`. */
    children: [ReactNode, ReactNode];
}

function readPersisted(key: string | undefined, fallback: number): number {
    if (!key) return fallback;
    try {
        const raw = localStorage.getItem(STORAGE_PREFIX + key);
        if (raw == null) return fallback;
        const n = parseFloat(raw);
        if (!Number.isFinite(n) || n < 0 || n > 100) return fallback;
        return n;
    } catch {
        return fallback;
    }
}

function writePersisted(key: string | undefined, pct: number) {
    if (!key) return;
    try {
        localStorage.setItem(STORAGE_PREFIX + key, String(pct));
    } catch {
        // best-effort — quota errors etc. just drop persistence.
    }
}

export default function Split({
    direction = "row",
    defaultPercent = 50,
    minPercent = 10,
    maxPercent = 90,
    storageKey,
    className,
    children,
}: SplitProps) {
    const [first, second] = Children.toArray(children);
    const containerRef = useRef<HTMLDivElement>(null);
    const [pct, setPct] = useState<number>(() =>
        readPersisted(storageKey, defaultPercent),
    );
    // Mirror the latest pct into a ref so the pointermove handler
    // doesn't have to re-bind whenever pct changes.
    const pctRef = useRef(pct);
    pctRef.current = pct;

    const horizontal = direction === "row";

    const onPointerDown = useCallback(
        (event: React.PointerEvent<HTMLDivElement>) => {
            event.preventDefault();
            const container = containerRef.current;
            if (!container) return;
            const seam = event.currentTarget;
            seam.setPointerCapture(event.pointerId);

            const onMove = (ev: PointerEvent) => {
                const r = container.getBoundingClientRect();
                const span = horizontal ? r.width : r.height;
                if (span <= 0) return;
                const offset = horizontal ? ev.clientX - r.left : ev.clientY - r.top;
                const next = Math.max(
                    minPercent,
                    Math.min(maxPercent, (offset / span) * 100),
                );
                setPct(next);
                writePersisted(storageKey, next);
            };
            const onUp = (ev: PointerEvent) => {
                if (seam.hasPointerCapture(ev.pointerId)) {
                    seam.releasePointerCapture(ev.pointerId);
                }
                window.removeEventListener("pointermove", onMove);
                window.removeEventListener("pointerup", onUp);
                window.removeEventListener("pointercancel", onUp);
            };
            window.addEventListener("pointermove", onMove);
            window.addEventListener("pointerup", onUp);
            window.addEventListener("pointercancel", onUp);
        },
        [horizontal, minPercent, maxPercent, storageKey],
    );

    // Cross-tab/cross-mount sync via the `storage` event.
    useEffect(() => {
        if (!storageKey) return;
        const fullKey = STORAGE_PREFIX + storageKey;
        const onStorage = (e: StorageEvent) => {
            if (e.key !== fullKey || e.newValue == null) return;
            const n = parseFloat(e.newValue);
            if (Number.isFinite(n) && n >= 0 && n <= 100 && n !== pctRef.current) {
                setPct(n);
            }
        };
        window.addEventListener("storage", onStorage);
        return () => window.removeEventListener("storage", onStorage);
    }, [storageKey]);

    return (
        <div
            ref={containerRef}
            className={cn("flex min-h-0 min-w-0", className)}
            style={{ flexDirection: direction }}
            data-split-direction={direction}
        >
            <div
                className="min-h-0 min-w-0 overflow-hidden"
                style={{ flex: `${pct} 1 0%` }}
            >
                {first}
            </div>
            <div
                role="separator"
                aria-orientation={horizontal ? "vertical" : "horizontal"}
                aria-valuemin={minPercent}
                aria-valuemax={maxPercent}
                aria-valuenow={Math.round(pct)}
                onPointerDown={onPointerDown}
                className={cn(
                    "relative shrink-0 bg-border touch-none",
                    horizontal
                        ? "w-px cursor-col-resize"
                        : "h-px cursor-row-resize",
                    // Wide invisible hit-area so the 1-px seam is
                    // grabbable. The visible bar stays 1 px;
                    // hover/focus just colour it differently.
                    "hover:bg-primary/40",
                    "after:absolute after:bg-transparent",
                    horizontal
                        ? "after:inset-y-0 after:-inset-x-1"
                        : "after:inset-x-0 after:-inset-y-1",
                )}
            />
            <div
                className="min-h-0 min-w-0 overflow-hidden"
                style={{ flex: `${100 - pct} 1 0%` }}
            >
                {second}
            </div>
        </div>
    );
}
