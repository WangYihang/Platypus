// Web-mode drop-in for wailsjs/runtime/runtime. Pages import EventsOn /
// EventsOff to subscribe to server-pushed events (notify:* + terminal:*).
// This shim backs them with a browser EventTarget so any producer — the
// REST shim in App.web.ts (for terminal:* during W3) or a /notify
// WebSocket (deferred to Phase 2) — can emit by calling emitEvent.

type Handler = (...args: any[]) => void;

const bus = new EventTarget();
// Track all handlers keyed by event name so EventsOff can remove them
// without callers holding a reference to the specific Handler.
const handlers = new Map<string, Set<EventListener>>();

export function EventsOn(name: string, fn: Handler): void {
    const listener: EventListener = (ev) => {
        const detail = (ev as CustomEvent).detail;
        if (Array.isArray(detail)) {
            fn(...detail);
        } else {
            fn(detail);
        }
    };
    if (!handlers.has(name)) handlers.set(name, new Set());
    handlers.get(name)!.add(listener);
    bus.addEventListener(name, listener);
}

export function EventsOff(name: string, ..._fns: Handler[]): void {
    const set = handlers.get(name);
    if (!set) return;
    for (const l of set) bus.removeEventListener(name, l);
    handlers.delete(name);
}

// emitEvent is the producer side — used by App.web.ts for terminal:output
// frames, and (Phase 2) the /notify subscriber for notify:* events.
export function emitEvent(name: string, ...args: any[]): void {
    const detail = args.length === 1 ? args[0] : args;
    bus.dispatchEvent(new CustomEvent(name, { detail }));
}

// Wails also exports these names; pages don't use them today but keeping
// them avoids import errors if a future page subscribes to window events.
export function WindowMinimise(): void {}
export function WindowReload(): void {
    window.location.reload();
}
