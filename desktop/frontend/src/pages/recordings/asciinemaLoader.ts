// Lazy loader for asciinema-player. The library + its embedded WASM
// VT module + CSS together add ~150KB gzipped, so we load them with a
// dynamic import on first preview rather than paying for them in the
// main app chunk. Vite splits this into its own chunk and the CSS
// import side-effect is hoisted into that same chunk's stylesheet,
// so subsequent previews resolve from cache.
//
// We deliberately do NOT load from a CDN. The desktop build runs
// inside a Wails webview that may have no outbound network (offline
// or air-gapped deployments) or a CSP that blocks external scripts —
// either of which silently breaks playback (the play button renders
// but click does nothing because driver init fails). Bundling with
// Vite makes recording playback work in every environment the rest
// of the app works in.

interface PlayerOptions {
    cols?: number;
    rows?: number;
    autoPlay?: boolean;
    loop?: boolean;
    speed?: number;
    idleTimeLimit?: number;
    theme?: string;
    poster?: string;
    fit?: "width" | "height" | "both" | "false" | false | undefined;
    terminalFontSize?: string;
    terminalLineHeight?: number;
}

export interface AsciinemaPlayerAPI {
    create(
        src: string | { url: string } | { data: string | Response | unknown[] },
        target: HTMLElement,
        opts?: PlayerOptions,
    ): { dispose?: () => void };
}

let loadPromise: Promise<AsciinemaPlayerAPI> | null = null;

// loadAsciinemaPlayer returns the asciinema-player module's `create`
// API once the JS chunk + CSS are loaded. Cached: subsequent calls
// resolve immediately.
export function loadAsciinemaPlayer(): Promise<AsciinemaPlayerAPI> {
    if (typeof window === "undefined") {
        return Promise.reject(new Error("asciinema-player requires a browser"));
    }
    if (loadPromise) return loadPromise;

    loadPromise = Promise.all([
        import("asciinema-player"),
        // Side-effect CSS import; Vite emits it as a stylesheet asset
        // alongside the lazy JS chunk.
        import("asciinema-player/dist/bundle/asciinema-player.css"),
    ]).then(([mod]) => ({ create: mod.create } as AsciinemaPlayerAPI));
    return loadPromise;
}
