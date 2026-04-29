/// <reference types="vite/client" />

declare const __APP_VERSION__: string;
declare const __APP_COMMIT__: string;

// asciinema-player ships no TypeScript types; we only consume its
// `create` factory and the per-instance dispose handle.
declare module "asciinema-player" {
    export function create(
        src: unknown,
        elem: HTMLElement,
        opts?: Record<string, unknown>,
    ): { dispose?: () => void };
}

declare module "asciinema-player/dist/bundle/asciinema-player.css";
