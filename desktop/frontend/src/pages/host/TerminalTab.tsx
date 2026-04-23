import { useCallback, useEffect, useRef, useState } from "react";
import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import { font, palette, radius, space } from "../../layout/theme";
import { SessionRow } from "../../lib/api";
import Terminal from "../Terminal";

interface Props {
    liveSessions: SessionRow[];
    picked: string | null;
    onPick: (sessionHash: string) => void;
}

interface ShellTab {
    id: string;
    label: string;
    sessionHash: string;
}

let shellCounter = 0;

export default function TerminalTab({ liveSessions, picked, onPick }: Props) {
    const [tabs, setTabs] = useState<ShellTab[]>([]);
    const [activeTabId, setActiveTabId] = useState<string | null>(null);
    const closedTabsRef = useRef(new Set<string>());

    const openShell = useCallback(() => {
        if (!picked) return;
        shellCounter += 1;
        const tab: ShellTab = {
            id: `shell-${Date.now()}-${shellCounter}`,
            label: `Shell ${shellCounter}`,
            sessionHash: picked,
        };
        setTabs((prev) => [...prev, tab]);
        setActiveTabId(tab.id);
    }, [picked]);

    // Automatically open first shell if none exist and we have a picked session.
    // Use an effect so it runs after the initial mount / session resolution.
    useEffect(() => {
        if (picked && tabs.length === 0 && !closedTabsRef.current.size) {
            openShell();
        }
    }, [picked, tabs.length, openShell]);

    const closeTab = useCallback(
        (tabId: string) => {
            closedTabsRef.current.add(tabId);
            setTabs((prev) => {
                const next = prev.filter((t) => t.id !== tabId);
                if (activeTabId === tabId) {
                    setActiveTabId(next.length > 0 ? next[next.length - 1].id : null);
                }
                return next;
            });
        },
        [activeTabId],
    );

    if (liveSessions.length === 0) {
        return (
            <EmptyState
                title="No live session"
                description="Waiting for the agent to reconnect to a listener."
            />
        );
    }

    const showPicker = liveSessions.length > 1;

    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                height: "100%",
                gap: space[3],
            }}
        >
            {showPicker && (
                <div style={{ display: "flex", flexWrap: "wrap", gap: space[2] }}>
                    {liveSessions.map((s) => {
                        const selected = s.id === picked;
                        return (
                            <button
                                key={s.id}
                                onClick={() => onPick(s.id)}
                                style={{
                                    display: "inline-flex",
                                    alignItems: "center",
                                    gap: space[2],
                                    padding: `4px ${space[3]}px`,
                                    background: selected
                                        ? palette.surfaceHover
                                        : "transparent",
                                    border: `1px solid ${
                                        selected ? palette.textPrimary : palette.border
                                    }`,
                                    borderRadius: radius.md,
                                    color: palette.textPrimary,
                                    fontFamily: font.mono,
                                    fontSize: 12,
                                    fontWeight: 500,
                                    cursor: "pointer",
                                    transition: "border-color 120ms ease",
                                }}
                            >
                                <Mono size={12}>{s.id.slice(0, 12)}</Mono>
                                {s.user && (
                                    <span
                                        style={{
                                            color: palette.textSecondary,
                                            fontFamily: font.sans,
                                            fontSize: 11,
                                        }}
                                    >
                                        · {s.user}
                                    </span>
                                )}
                            </button>
                        );
                    })}
                </div>
            )}

            {/* Shell tab bar */}
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 2,
                    minHeight: 32,
                }}
            >
                {tabs.map((tab) => {
                    const active = tab.id === activeTabId;
                    return (
                        <div
                            key={tab.id}
                            style={{
                                display: "inline-flex",
                                alignItems: "center",
                                gap: 4,
                                padding: `4px ${space[3]}px`,
                                background: active ? palette.surfaceHover : "transparent",
                                border: `1px solid ${active ? palette.textPrimary : palette.border}`,
                                borderRadius: `${radius.md}px ${radius.md}px 0 0`,
                                color: palette.textPrimary,
                                fontFamily: font.mono,
                                fontSize: 12,
                                cursor: "pointer",
                                transition: "border-color 120ms ease",
                            }}
                        >
                            <span onClick={() => setActiveTabId(tab.id)}>
                                {tab.label}
                            </span>
                            <button
                                onClick={(e) => {
                                    e.stopPropagation();
                                    closeTab(tab.id);
                                }}
                                style={{
                                    background: "none",
                                    border: "none",
                                    color: palette.textMuted,
                                    cursor: "pointer",
                                    padding: "0 2px",
                                    fontSize: 14,
                                    lineHeight: 1,
                                }}
                                title="Close shell"
                            >
                                ×
                            </button>
                        </div>
                    );
                })}
                <button
                    onClick={openShell}
                    disabled={!picked}
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        justifyContent: "center",
                        width: 28,
                        height: 28,
                        background: "transparent",
                        border: `1px solid ${palette.border}`,
                        borderRadius: radius.md,
                        color: palette.textSecondary,
                        cursor: picked ? "pointer" : "not-allowed",
                        fontSize: 16,
                        lineHeight: 1,
                        opacity: picked ? 1 : 0.4,
                        transition: "border-color 120ms ease",
                    }}
                    title="Open new shell"
                >
                    +
                </button>
            </div>

            {/* Terminal area */}
            <div
                style={{
                    flex: 1,
                    minHeight: 320,
                    border: `1px solid ${palette.border}`,
                    borderRadius: radius.md,
                    overflow: "hidden",
                    background: palette.main,
                }}
            >
                {tabs.length === 0 ? (
                    <EmptyState
                        fill
                        title="No shells open"
                        description="Click + to open a new shell session."
                    />
                ) : (
                    tabs.map((tab) => (
                        <div
                            key={tab.id}
                            style={{
                                display: tab.id === activeTabId ? "block" : "none",
                                height: "100%",
                                width: "100%",
                            }}
                        >
                            <Terminal
                                sessionHash={tab.sessionHash}
                                onClose={() => closeTab(tab.id)}
                            />
                        </div>
                    ))
                )}
            </div>
        </div>
    );
}
