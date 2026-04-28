// Lazy loader for asciinema-player. We avoid bundling it as an npm
// dependency so the main app chunk stays small (the player ships its
// own xterm-style renderer + CSS that adds ~150KB gzipped). On first
// preview the player JS + CSS are pulled from the configurable CDN
// origin, cached on `window`, and reused for every subsequent open.
//
// The CDN origin can be overridden at runtime via
// localStorage["platypus.asciinemaCDN"] for offline / air-gapped
// deployments — point it at a self-hosted mirror that exposes the
// `dist/` folder of the asciinema-player release.

const DEFAULT_CDN = "https://cdn.jsdelivr.net/npm/asciinema-player@3.10.0";

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

interface AsciinemaPlayerAPI {
    create(
        src: string | { url: string } | { data: string | unknown[] },
        target: HTMLElement,
        opts?: PlayerOptions,
    ): { dispose?: () => void };
}

declare global {
    interface Window {
        AsciinemaPlayer?: AsciinemaPlayerAPI;
    }
}

let loadPromise: Promise<AsciinemaPlayerAPI> | null = null;

function cdnBase(): string {
    if (typeof window === "undefined") return DEFAULT_CDN;
    try {
        const override = window.localStorage?.getItem("platypus.asciinemaCDN");
        if (override) return override.replace(/\/$/, "");
    } catch {
        // storage disabled in some sandboxed iframes — fall through.
    }
    return DEFAULT_CDN;
}

function injectCSS(href: string) {
    if (document.querySelector(`link[data-platypus-asciinema-css="1"]`)) return;
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    link.dataset.platypusAsciinemaCss = "1";
    document.head.appendChild(link);
}

function injectScript(src: string): Promise<void> {
    return new Promise((resolve, reject) => {
        const existing = document.querySelector(
            `script[data-platypus-asciinema-js="1"]`,
        ) as HTMLScriptElement | null;
        if (existing) {
            if (existing.dataset.loaded === "1") {
                resolve();
                return;
            }
            existing.addEventListener("load", () => resolve());
            existing.addEventListener("error", () =>
                reject(new Error("asciinema-player script failed to load")),
            );
            return;
        }
        const s = document.createElement("script");
        s.src = src;
        s.async = true;
        s.dataset.platypusAsciinemaJs = "1";
        s.addEventListener("load", () => {
            s.dataset.loaded = "1";
            resolve();
        });
        s.addEventListener("error", () =>
            reject(new Error("asciinema-player script failed to load")),
        );
        document.head.appendChild(s);
    });
}

// loadAsciinemaPlayer returns the global API once the script + CSS have
// been pulled in. Cached: subsequent calls resolve immediately.
export function loadAsciinemaPlayer(): Promise<AsciinemaPlayerAPI> {
    if (typeof window === "undefined") {
        return Promise.reject(new Error("asciinema-player requires a browser"));
    }
    if (window.AsciinemaPlayer) {
        return Promise.resolve(window.AsciinemaPlayer);
    }
    if (loadPromise) return loadPromise;

    const base = cdnBase();
    injectCSS(`${base}/dist/bundle/asciinema-player.css`);
    loadPromise = injectScript(`${base}/dist/bundle/asciinema-player.min.js`).then(() => {
        if (!window.AsciinemaPlayer) {
            throw new Error("asciinema-player loaded but global is missing");
        }
        return window.AsciinemaPlayer;
    });
    return loadPromise;
}
