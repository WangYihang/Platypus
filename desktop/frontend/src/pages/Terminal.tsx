import { useEffect, useRef } from "react";
import { Terminal as Xterm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

import {
    CloseTerminal,
    OpenTerminal,
    ResizeTerminal,
    SendTerminalInput,
} from "../../wailsjs/go/app/App";
import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";

interface Props {
    sessionHash: string;
    onClose?: () => void;
}

// One <Terminal> per open session tab. Owns the xterm instance and the
// underlying termID returned from OpenTerminal; reroutes Wails events to
// xterm.write() and xterm.onData → SendTerminalInput.
export default function Terminal({ sessionHash, onClose }: Props) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const xtermRef = useRef<Xterm | null>(null);
    const fitRef = useRef<FitAddon | null>(null);
    const termIDRef = useRef<string>("");

    useEffect(() => {
        if (!containerRef.current) return;

        const xterm = new Xterm({
            fontFamily: "Menlo, Consolas, 'Liberation Mono', monospace",
            fontSize: 13,
            theme: { background: "#1e1e1e", foreground: "#d4d4d4" },
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

        OpenTerminal(sessionHash)
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
                const onResize = () => {
                    fit.fit();
                    ResizeTerminal(id, xterm.cols, xterm.rows);
                };
                window.addEventListener("resize", onResize);
                cleanupFns.push(() => window.removeEventListener("resize", onResize));

                EventsOn(`terminal:output:${id}`, (b64: string) => {
                    try {
                        xterm.write(atob(b64));
                    } catch (e) {
                        xterm.write(`\r\n[decode err: ${String(e)}]\r\n`);
                    }
                });
                EventsOn(`terminal:closed:${id}`, () => {
                    xterm.write("\r\n[connection closed]\r\n");
                    onClose?.();
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
    }, [sessionHash, onClose]);

    return (
        <div
            ref={containerRef}
            style={{ height: "100%", width: "100%", background: "#1e1e1e" }}
        />
    );
}
