import {
    DragEvent,
    ReactNode,
    useCallback,
    useRef,
    useState,
} from "react";

import { palette, radius, space } from "../layout/theme";

interface Props {
    children: ReactNode;
    /**
     * Fired when files are dropped onto the zone. Receives a real
     * `File[]` (already extracted from DataTransfer.files) so the
     * caller doesn't need to know about the DnD API. Empty drops
     * (no files in the transfer) skip the callback.
     */
    onDrop: (files: File[]) => void;
    /** When true the overlay never appears and drops are ignored. */
    disabled?: boolean;
}

/**
 * FileDropZone wraps its children with a drag-and-drop file overlay.
 * Drag-enter shows a dashed-border overlay over the entire wrapped
 * region; dropping invokes onDrop with the File[] from
 * dataTransfer.files. Drags that don't carry files (text selection,
 * tab reorder, etc.) are ignored — see the dataTransfer.types check.
 *
 * Implementation notes:
 *   * dragEnter / dragLeave bookkeeping uses a counter rather than
 *     a single boolean — each child element fires its own
 *     enter/leave when the cursor crosses its boundary, so a naive
 *     boolean would flicker the overlay off as the cursor moves
 *     between children. The counter goes up on enter, down on
 *     leave; overlay is visible whenever counter > 0.
 *   * preventDefault on dragOver is mandatory — without it the
 *     browser's default "open this file in a new tab" behavior
 *     wins on drop and our handler never fires.
 */
export default function FileDropZone({ children, onDrop, disabled }: Props) {
    const [active, setActive] = useState(false);
    const counter = useRef(0);

    const isFileDrag = useCallback((ev: DragEvent) => {
        const types = ev.dataTransfer?.types;
        if (!types) return false;
        // types is a DOMStringList in some browsers — check both shapes.
        for (let i = 0; i < types.length; i++) {
            if (types[i] === "Files") return true;
        }
        return false;
    }, []);

    const handleEnter = useCallback(
        (ev: DragEvent) => {
            if (disabled) return;
            if (!isFileDrag(ev)) return;
            ev.preventDefault();
            counter.current += 1;
            setActive(true);
        },
        [disabled, isFileDrag],
    );

    const handleOver = useCallback(
        (ev: DragEvent) => {
            if (disabled) return;
            if (!isFileDrag(ev)) return;
            // Required to fire onDrop on the same element. Without
            // preventDefault here, the browser falls back to default
            // file-handling and our drop handler is never called.
            ev.preventDefault();
            if (ev.dataTransfer) {
                ev.dataTransfer.dropEffect = "copy";
            }
        },
        [disabled, isFileDrag],
    );

    const handleLeave = useCallback(
        (ev: DragEvent) => {
            if (disabled) return;
            if (!isFileDrag(ev)) return;
            counter.current = Math.max(0, counter.current - 1);
            if (counter.current === 0) setActive(false);
        },
        [disabled, isFileDrag],
    );

    const handleDrop = useCallback(
        (ev: DragEvent) => {
            if (disabled) return;
            if (!isFileDrag(ev)) return;
            ev.preventDefault();
            counter.current = 0;
            setActive(false);
            const files: File[] = [];
            const list = ev.dataTransfer?.files;
            if (list) {
                for (let i = 0; i < list.length; i++) {
                    const f = list.item(i);
                    if (f) files.push(f);
                }
            }
            if (files.length > 0) onDrop(files);
        },
        [disabled, isFileDrag, onDrop],
    );

    return (
        <div
            onDragEnter={handleEnter}
            onDragOver={handleOver}
            onDragLeave={handleLeave}
            onDrop={handleDrop}
            style={{ position: "relative", height: "100%" }}
        >
            {children}
            {active ? (
                <div
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        background: "rgba(0, 112, 243, 0.08)",
                        border: `2px dashed ${palette.info}`,
                        borderRadius: radius.md,
                        pointerEvents: "none",
                        zIndex: 50,
                    }}
                >
                    <div
                        style={{
                            background: palette.surface,
                            border: `1px solid ${palette.border}`,
                            borderRadius: radius.md,
                            padding: `${space[3]}px ${space[5]}px`,
                            color: palette.textPrimary,
                            fontSize: 13,
                            fontWeight: 500,
                        }}
                    >
                        Drop files to upload
                    </div>
                </div>
            ) : null}
        </div>
    );
}
