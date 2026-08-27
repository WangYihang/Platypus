import type { Extension } from "@codemirror/state";
import { useQuery } from "@tanstack/react-query";

import { inferLanguage } from "./paths";

/**
 * Lazily loads the CodeMirror grammar for a file, keyed on the
 * language rather than the path.
 *
 * The editor did this in an effect: null the extension, run a switch
 * of dynamic imports, and set the result unless a cancelled flag had
 * been flipped. Two things were wrong with that. It is a pure
 * "language in, extension out" lookup, so it does not need a
 * cancellation dance — and it was keyed on the path, so opening a
 * second .py file re-ran the whole thing for a grammar already in
 * memory.
 *
 * The variants CodeMirror wants for one grammar (jsx / typescript)
 * are part of the key, so .ts and .tsx get their own entries instead
 * of sharing one and picking up whichever loaded first.
 */
async function loadExtension(lang: string, jsx: boolean, typescript: boolean): Promise<Extension | null> {
    switch (lang) {
        case "json": {
            const m = await import("@codemirror/lang-json");
            return m.json();
        }
        case "javascript": {
            const m = await import("@codemirror/lang-javascript");
            return m.javascript({ jsx, typescript });
        }
        case "python": {
            const m = await import("@codemirror/lang-python");
            return m.python();
        }
        case "shell": {
            const m = await import("@codemirror/legacy-modes/mode/shell");
            const { StreamLanguage } = await import("@codemirror/language");
            return StreamLanguage.define(m.shell);
        }
        default:
            return null;
    }
}

export function useLanguageExtension(path: string): Extension | null {
    const lang = inferLanguage(path);
    const jsx = /\.(jsx|tsx)$/i.test(path);
    const typescript = /\.tsx?$/i.test(path);

    const { data } = useQuery({
        queryKey: ["codemirrorLanguage", lang, jsx, typescript] as const,
        queryFn: () => loadExtension(lang, jsx, typescript),
        // A grammar module does not change under us.
        staleTime: Infinity,
        gcTime: Infinity,
        refetchOnWindowFocus: false,
    });
    return data ?? null;
}
