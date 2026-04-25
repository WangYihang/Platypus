import { useEffect, useRef, useState } from "react";

import { EventsOff, EventsOn } from "../../../../wailsjs/runtime/runtime";
import { UploadFile } from "../../../../wailsjs/go/app/App";
import { UploadBrowserFile } from "../../../platform/App.web";
import { joinPath } from "./paths";

// UploadProgress is a lightweight event the browser UI can subscribe to
// so we render a toast or overlay while dropped files are transferring.
export interface UploadProgress {
    filename: string;
    done: number;
    total: number;
    error?: string;
}

// Desktop path: Wails' OS-drop bridge emits "files:os-drop" with
// { paths: string[] }. We translate each absolute path into an
// UploadFile call targeting the currently-browsed remote directory.
// Web path: the browser lets us register HTML5 dragover/drop on any
// element. The returned helpers wire those for us.

interface UseDragDropOpts {
    projectID: string;
    sessionHash: string;
    currentPath: string;
    onFinished?: () => void;
    onError?: (err: string) => void;
    onProgress?: (p: UploadProgress) => void;
}

export function useDragDrop({
    projectID,
    sessionHash,
    currentPath,
    onFinished,
    onError,
    onProgress,
}: UseDragDropOpts) {
    const [dragOver, setDragOver] = useState(false);
    // Keep the latest path in a ref so the event listeners (registered
    // once) always upload into the directory the user is *currently*
    // viewing, not whatever path was live when the subscription opened.
    const pathRef = useRef(currentPath);
    pathRef.current = currentPath;

    // --- Desktop: Wails OS drop --------------------------------------
    useEffect(() => {
        const handler = async (payload: { paths?: string[] }) => {
            const paths = payload?.paths || [];
            if (paths.length === 0) return;
            for (let i = 0; i < paths.length; i++) {
                const p = paths[i];
                const filename = basenameOSPath(p);
                onProgress?.({ filename, done: i, total: paths.length });
                try {
                    await UploadFile(projectID, sessionHash, joinPath(pathRef.current, filename), p);
                } catch (err) {
                    const msg = String(err instanceof Error ? err.message : err);
                    onError?.(`upload ${filename}: ${msg}`);
                    onProgress?.({ filename, done: i, total: paths.length, error: msg });
                    return;
                }
            }
            onProgress?.({ filename: "", done: paths.length, total: paths.length });
            onFinished?.();
        };
        EventsOn("files:os-drop", handler);
        return () => {
            EventsOff("files:os-drop");
        };
    }, [projectID, sessionHash, onFinished, onError, onProgress]);

    // --- Web: HTML5 drop handlers — returned so a container element
    // can spread them on.
    const dropHandlers = {
        onDragOver: (e: React.DragEvent) => {
            if (e.dataTransfer?.types?.includes("Files")) {
                e.preventDefault();
                setDragOver(true);
            }
        },
        onDragLeave: (e: React.DragEvent) => {
            if (e.currentTarget === e.target) setDragOver(false);
        },
        onDrop: async (e: React.DragEvent) => {
            e.preventDefault();
            setDragOver(false);
            const files = Array.from(e.dataTransfer?.files || []);
            if (files.length === 0) return;
            for (let i = 0; i < files.length; i++) {
                const f = files[i];
                onProgress?.({ filename: f.name, done: i, total: files.length });
                try {
                    await UploadBrowserFile(
                        projectID,
                        sessionHash,
                        joinPath(pathRef.current, f.name),
                        f,
                    );
                } catch (err) {
                    const msg = String(err instanceof Error ? err.message : err);
                    onError?.(`upload ${f.name}: ${msg}`);
                    onProgress?.({ filename: f.name, done: i, total: files.length, error: msg });
                    return;
                }
            }
            onProgress?.({ filename: "", done: files.length, total: files.length });
            onFinished?.();
        },
    };

    return { dragOver, dropHandlers };
}

// basenameOSPath tolerates either / or \ separators — Wails on Windows
// hands us backslash paths from File Explorer.
function basenameOSPath(p: string): string {
    const i = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
    return i >= 0 ? p.slice(i + 1) : p;
}
