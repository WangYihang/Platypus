import { useCallback, useEffect, useMemo, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { EditorView } from "@codemirror/view";
import { EditorState, Extension } from "@codemirror/state";
import { oneDark } from "@codemirror/theme-one-dark";
import { Loader2, Save } from "lucide-react";
import { toast } from "sonner";
import { humanizeError } from "../../../lib/humanizeError";

import { Button } from "@/components/ui/button";
import { WriteFile } from "@wails/go/app/App";
import { humanize } from "../../../lib/format";
import { useOnValueChange } from "@/lib/useOnValueChange";
import { useRemoteFileText } from "./remoteFile";
import { inferLanguage } from "./paths";

// SMALL_FILE_LIMIT is the in-memory edit threshold. Below: full load,
// edit, save. Above: caller should route to FileViewerPaged instead.
export const SMALL_FILE_LIMIT = 5 * 1024 * 1024;

// WRITE_CHUNK matches the server's existing 256 KiB contract. The first
// chunk overwrites; subsequent ones append.
const WRITE_CHUNK = 256 * 1024;

interface Props {
    projectID: string;
    sessionHash: string;
    path: string;
    size: number;
    onSaved?: () => void;
}

export default function FileEditor({ projectID, sessionHash, path, size, onSaved }: Props) {
    const [content, setContent] = useState("");
    const [dirty, setDirty] = useState(false);
    const [saving, setSaving] = useState(false);
    const [langExt, setLangExt] = useState<Extension | null>(null);

    // The read is a query keyed on the path, so the stale-response
    // problem it used to guard against with a pathRef — open a file,
    // open another before the first lands, watch the first overwrite
    // the editor — cannot arise: a response only ever populates the
    // key it was requested for.
    const file = useRemoteFileText({ projectID, sessionHash, path });
    const loading = file.isFetching;
    const loaded = file.text !== null;
    const loadError = file.error
        ? String(file.error instanceof Error ? file.error.message : file.error)
        : null;

    // Seeded, not derived: this is the editor buffer, so once the
    // bytes land the operator owns it. Re-seeding on each arrival is
    // what "reload discards my edits" means, which is the behaviour
    // the old effect had via setContent + setDirty(false).
    useOnValueChange(file.text, (text) => {
        if (text === null) return;
        setContent(text);
        setDirty(false);
    });

    // Lazy-load the language extension per file type. Keeps the initial
    // editor bundle minimal.
    useEffect(() => {
        let cancelled = false;
        const lang = inferLanguage(path);
        setLangExt(null);
        void (async () => {
            let ext: Extension | null = null;
            switch (lang) {
                case "json": {
                    const m = await import("@codemirror/lang-json");
                    ext = m.json();
                    break;
                }
                case "javascript": {
                    const m = await import("@codemirror/lang-javascript");
                    ext = m.javascript({ jsx: /\.(jsx|tsx)$/i.test(path), typescript: /\.tsx?$/i.test(path) });
                    break;
                }
                case "python": {
                    const m = await import("@codemirror/lang-python");
                    ext = m.python();
                    break;
                }
                case "shell": {
                    const m = await import("@codemirror/legacy-modes/mode/shell");
                    const { StreamLanguage } = await import("@codemirror/language");
                    ext = StreamLanguage.define(m.shell);
                    break;
                }
                default:
                    ext = null;
            }
            if (!cancelled) setLangExt(ext);
        })();
        return () => {
            cancelled = true;
        };
    }, [path]);

    // Browser-native guard — warns if the tab/window is closed with
    // unsaved edits. Does not fire for intra-app router navigation; the
    // parent can layer its own guard on top if needed.
    useEffect(() => {
        if (!dirty) return;
        const handler = (ev: BeforeUnloadEvent) => {
            ev.preventDefault();
            ev.returnValue = "";
        };
        window.addEventListener("beforeunload", handler);
        return () => window.removeEventListener("beforeunload", handler);
    }, [dirty]);

    const handleChange = useCallback((next: string) => {
        setContent(next);
        setDirty(true);
    }, []);

    const save = useCallback(async () => {
        if (!dirty || saving) return;
        setSaving(true);
        try {
            const bytes = new TextEncoder().encode(content);
            // First chunk overwrites; subsequent append. Matches the
            // contract the existing UploadFile logic uses.
            if (bytes.length === 0) {
                await WriteFile(projectID, sessionHash, path, [], false);
            } else {
                for (let off = 0; off < bytes.length; off += WRITE_CHUNK) {
                    const slice = bytes.subarray(off, Math.min(off + WRITE_CHUNK, bytes.length));
                    await WriteFile(projectID, sessionHash, path, Array.from(slice), off > 0);
                }
            }
            setDirty(false);
            toast.success(`Saved ${path}`);
            onSaved?.();
        } catch (err) {
            toast.error(`save: ${humanizeError(err)}`);
        } finally {
            setSaving(false);
        }
    }, [content, dirty, saving, projectID, sessionHash, path, onSaved]);

    // Cmd/Ctrl+S keyboard shortcut inside the editor container.
    const keymapExt = useMemo<Extension>(
        () =>
            EditorView.domEventHandlers({
                keydown(event) {
                    if ((event.metaKey || event.ctrlKey) && event.key === "s") {
                        event.preventDefault();
                        void save();
                        return true;
                    }
                    return false;
                },
            }),
        [save],
    );

    const extensions = useMemo<Extension[]>(() => {
        const exts: Extension[] = [EditorState.allowMultipleSelections.of(true), keymapExt];
        if (langExt) exts.push(langExt);
        return exts;
    }, [langExt, keymapExt]);

    if (loading) {
        return (
            <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                Loading {humanize(size)}…
            </div>
        );
    }
    if (loadError) {
        return (
            <div className="flex h-full items-center justify-center text-sm text-red-500">
                Load error: {loadError}
            </div>
        );
    }
    if (!loaded) return null;

    return (
        <div className="flex h-full flex-col">
            <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                <div className="truncate font-mono">{path}</div>
                <div className="flex items-center gap-2">
                    {dirty && <span className="text-xs text-amber-500">unsaved</span>}
                    <Button type="button" size="sm" disabled={!dirty || saving} onClick={save}>
                        {saving ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <Save className="size-3.5" />
                        )}
                        Save
                    </Button>
                </div>
            </div>
            <div className="flex-1 overflow-hidden">
                <CodeMirror
                    value={content}
                    height="100%"
                    theme={oneDark}
                    extensions={extensions}
                    onChange={handleChange}
                    basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: true }}
                />
            </div>
        </div>
    );
}
