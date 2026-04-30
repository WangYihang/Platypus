import { useEffect, useRef } from "react";
import { Terminal as Xterm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

import {
    CloseTerminal,
    OpenTerminal,
    ResizeTerminal,
    SendTerminalInput,
} from "@wails/go/app/App";
import { EventsOff, EventsOn } from "@wails/runtime/runtime";
import { palette } from "../layout/theme";
import { readPreference } from "../lib/preferences";
import { useGlobalTerminalSafe } from "../terminal/GlobalTerminalContext";

interface Props {
    projectID: string;
    sessionHash: string;
    // Optional GlobalTerminalContext shell id. When set, the terminal
    // pulls a one-shot initial command (e.g. `cd /etc\n`) from the
    // context once the agent reports the session as open. Components
    // that mount Terminal outside the drawer (e.g. tests, future
    // standalone surfaces) can omit it.
    shellId?: string;
    onClose?: () => void;
}

// xterm theme aligned with Vercel neutral palette: pure-black background
// matches the page surface so the terminal blends into HostView's
// terminal tab instead of looking like an embedded gray box.
const xtermTheme = {
    background: palette.main,           // #000
    foreground: palette.textPrimary,    // #fafafa
    cursor: palette.textPrimary,
    cursorAccent: palette.main,
    selectionBackground: "#404040",
    black: "#000000",
    red: "#ff5c57",
    green: palette.info,
    yellow: "#f3f99d",
    blue: "#57c7ff",
    magenta: "#ff6ac1",
    cyan: "#9aedfe",
    white: "#f1f1f0",
    brightBlack: "#686868",
    brightRed: "#ff5c57",
    brightGreen: palette.info,
    brightYellow: "#f3f99d",
    brightBlue: "#57c7ff",
    brightMagenta: "#ff6ac1",
    brightCyan: "#9aedfe",
    brightWhite: "#ffffff",
};

// Minimum cols/rows we ever forward to the agent. xterm-fit reads
// `clientWidth` / `clientHeight` of the host element and divides by
// the rendered char dimensions; while the terminal drawer animates
// open (or while a parent panel resizes) the host is briefly very
// small, so fit() returns absurd dimensions like 9 cols × 7 rows.
// Sending those over the wire makes the agent's PTY actually resize
// to 9×7, which both wraps the live shell weirdly AND gets baked
// into the recorded .cast — playback then jumps to "huge text" mid-
// stream when the recording replays the bogus resize. The real
// dims always settle to something sensible once layout finishes,
// so we just suppress sub-threshold transients and let the next
// ResizeObserver fire deliver the steady-state value.
//
// 40 × 10 is well below the DEC VT100 24-row historical minimum but
// far above any layout glitch we've seen in the wild (9×7 was the
// reported case). Picking the floor too low lets the player still
// render "huge" text during the transient; too high and we'd lie
// to the agent about a genuinely-small operator terminal. 40×10
// keeps replay readable without inflating any realistic geometry.
const MIN_TERM_COLS = 40;
const MIN_TERM_ROWS = 10;

// One <Terminal> per open session tab. Owns the xterm instance and the
// underlying termID returned from OpenTerminal; reroutes Wails events to
// xterm.write() and xterm.onData → SendTerminalInput.
export default function Terminal({ projectID, sessionHash, shellId, onClose }: Props) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const xtermRef = useRef<Xterm | null>(null);
    const fitRef = useRef<FitAddon | null>(null);
    const termIDRef = useRef<string>("");
    // onClose is forwarded through a ref so re-renders that only change the
    // callback identity (e.g. parent state updates) don't re-run the xterm
    // setup effect and tear down the WebSocket.
    const onCloseRef = useRef(onClose);
    useEffect(() => {
        onCloseRef.current = onClose;
    }, [onClose]);

    const term = useGlobalTerminalSafe();
    // Stash the consumer in a ref so the xterm setup effect (which
    // depends only on projectID/sessionHash) doesn't re-run when the
    // terminal context's identity changes.
    const consumeInitialCommandRef = useRef(term?.consumeInitialCommand);
    useEffect(() => {
        consumeInitialCommandRef.current = term?.consumeInitialCommand;
    }, [term]);
    const shellIdRef = useRef(shellId);
    useEffect(() => {
        shellIdRef.current = shellId;
    }, [shellId]);

    useEffect(() => {
        if (!containerRef.current) return;

        // Pull terminal-shaped prefs from localStorage at construction
        // time so user changes in Settings → Terminal are reflected on
        // the next opened shell (re-mount of this component).
        const fontSize = readPreference("terminal.fontSize");
        const cursorBlink = readPreference("terminal.cursorBlink");
        const scrollback = readPreference("terminal.scrollback");

        const xterm = new Xterm({
            // Geist Mono lacks CJK / emoji glyphs, so the fallback
            // chain explicitly names the platform-default CJK
            // monospaces (PingFang on macOS, MS YaHei on Windows,
            // WenQuanYi / Noto on Linux). Without these the WebView
            // would draw tofu boxes for non-Latin terminal output.
            fontFamily:
                'var(--font-geist-mono), Menlo, Consolas, "Liberation Mono", ' +
                '"PingFang SC", "Microsoft YaHei", "Noto Sans Mono CJK SC", ' +
                '"WenQuanYi Micro Hei Mono", monospace',
            fontSize,
            lineHeight: 1.2,
            theme: xtermTheme,
            cursorBlink,
            scrollback,
            allowProposedApi: true,
        });
        const fit = new FitAddon();
        xterm.loadAddon(fit);
        xterm.open(containerRef.current);
        fit.fit();

        xtermRef.current = xterm;
        fitRef.current = fit;

        let cancelled = false;
        let cleanupFns: Array<() => void> = [];

        OpenTerminal(projectID, sessionHash)
            .then((id: string) => {
                if (cancelled) {
                    CloseTerminal(id);
                    return;
                }
                termIDRef.current = id;

                const encoder = new TextEncoder();
                xterm.onData((data) => {
                    SendTerminalInput(id, Array.from(encoder.encode(data)));
                });

                // The xterm viewport must track the container element, not
                // just the window: the terminal is hosted in a drag-resizable
                // drawer (TerminalDrawer) and in tabs whose layout can change
                // without firing window.resize. A ResizeObserver on the host
                // element catches every case; window resizes come along for
                // free because they also resize the container.
                let lastCols = xterm.cols;
                let lastRows = xterm.rows;
                let rafHandle: number | null = null;
                const applyResize = () => {
                    rafHandle = null;
                    try {
                        fit.fit();
                    } catch {
                        // Container has no layout yet (0×0); the observer
                        // will fire again once it does.
                        return;
                    }
                    if (xterm.cols === lastCols && xterm.rows === lastRows) {
                        return;
                    }
                    // Drop transient layout glitches (drawer animating
                    // open, parent panel resizing). See MIN_TERM_*
                    // comment above. Don't update lastCols/lastRows
                    // either — the next observer fire will compare
                    // against the previously valid steady-state value.
                    if (xterm.cols < MIN_TERM_COLS || xterm.rows < MIN_TERM_ROWS) {
                        return;
                    }
                    lastCols = xterm.cols;
                    lastRows = xterm.rows;
                    ResizeTerminal(id, xterm.cols, xterm.rows);
                };
                const scheduleResize = () => {
                    if (rafHandle !== null) return;
                    rafHandle = requestAnimationFrame(applyResize);
                };
                const ro = new ResizeObserver(scheduleResize);
                if (containerRef.current) {
                    ro.observe(containerRef.current);
                }
                cleanupFns.push(() => {
                    ro.disconnect();
                    if (rafHandle !== null) {
                        cancelAnimationFrame(rafHandle);
                    }
                });

                EventsOn(`terminal:output:${id}`, (b64: string) => {
                    try {
                        // Decode base64 → Uint8Array and hand the raw bytes
                        // to xterm. atob() returns a Latin-1 string where
                        // every byte becomes one codepoint, which mangles
                        // UTF-8 multi-byte sequences (CJK, emoji, line
                        // box-drawing) into mojibake. xterm.write accepts
                        // Uint8Array and runs its own UTF-8 decoder.
                        const binary = atob(b64);
                        const bytes = new Uint8Array(binary.length);
                        for (let i = 0; i < binary.length; i++) {
                            bytes[i] = binary.charCodeAt(i);
                        }
                        xterm.write(bytes);
                    } catch (e) {
                        xterm.write(`\r\n[decode err: ${String(e)}]\r\n`);
                    }
                });
                EventsOn(`terminal:closed:${id}`, () => {
                    xterm.write("\r\n[connection closed]\r\n");
                    onCloseRef.current?.();
                });
                cleanupFns.push(() => {
                    EventsOff(`terminal:output:${id}`);
                    EventsOff(`terminal:closed:${id}`);
                });

                // Trigger initial size sync now that the terminal is
                // open. The server's WS handler blocks reading the
                // first frame as a resize, so we MUST send something.
                // If layout hasn't settled yet (e.g. the drawer is
                // mid-animation when OpenTerminal resolves) we still
                // forward at least MIN_TERM_* — better than a 9×7
                // PTY; the next ResizeObserver fire delivers the
                // steady-state size.
                ResizeTerminal(
                    id,
                    Math.max(xterm.cols, MIN_TERM_COLS),
                    Math.max(xterm.rows, MIN_TERM_ROWS),
                );

                // Pull a one-shot seed command from the global
                // terminal context. The "Open in terminal here"
                // action in the file browser sets this to a
                // `cd /some/path\n` so the operator lands directly
                // in the directory they were just browsing instead
                // of having to retype the cd by hand.
                const sid = shellIdRef.current;
                const consume = consumeInitialCommandRef.current;
                if (sid && consume) {
                    const cmd = consume(sid);
                    if (cmd) {
                        SendTerminalInput(id, Array.from(encoder.encode(cmd)));
                    }
                }
            })
            .catch((err: unknown) => {
                xterm.write(`\r\n[failed to open: ${String(err)}]\r\n`);
            });

        return () => {
            cancelled = true;
            cleanupFns.forEach((fn) => fn());
            const id = termIDRef.current;
            if (id) {
                CloseTerminal(id).catch(() => {});
            }
            xterm.dispose();
        };
    }, [projectID, sessionHash]);

    return (
        <div
            ref={containerRef}
            style={{ height: "100%", width: "100%", background: palette.main }}
        />
    );
}
