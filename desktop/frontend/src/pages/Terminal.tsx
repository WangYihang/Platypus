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

interface Props {
    projectID: string;
    sessionHash: string;
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

// One <Terminal> per open session tab. Owns the xterm instance and the
// underlying termID returned from OpenTerminal; reroutes Wails events to
// xterm.write() and xterm.onData → SendTerminalInput.
export default function Terminal({ projectID, sessionHash, onClose }: Props) {
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

    useEffect(() => {
        if (!containerRef.current) return;

        const xterm = new Xterm({
            fontFamily:
                'var(--font-geist-mono), Menlo, Consolas, "Liberation Mono", monospace',
            fontSize: 13,
            lineHeight: 1.2,
            theme: xtermTheme,
            cursorBlink: true,
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
                        xterm.write(atob(b64));
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

                // Trigger initial size sync now that the terminal is open.
                ResizeTerminal(id, xterm.cols, xterm.rows);
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
