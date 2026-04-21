import { useState } from "react";
import { CaretDownOutlined, CaretRightOutlined, ThunderboltOutlined } from "@ant-design/icons";

import { Host, Listener, Project } from "../lib/api";
import { palette } from "./theme";
import type { Selection } from "./Sidebar";
import ListenerRow from "./ListenerRow";
import HostRow from "./HostRow";

interface Props {
    project: Project;
    listeners: Listener[];
    hosts: Host[];
    selection: Selection | null;
    onSelect: (s: Selection) => void;
}

// ProjectSection is a Slack-workspace-shaped block: project header,
// then a LISTENERS sub-section and a HOSTS sub-section, then a Dispatch
// row as the per-project action. Both sub-sections are independently
// collapsible so an operator with a huge fleet can hide the host list
// without losing the listener view.
export default function ProjectSection({
    project,
    listeners,
    hosts,
    selection,
    onSelect,
}: Props) {
    const [open, setOpen] = useState(true);
    const [listenersOpen, setListenersOpen] = useState(true);
    const [hostsOpen, setHostsOpen] = useState(true);

    return (
        <div style={{ padding: "0 8px 12px" }}>
            <header
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 6,
                    padding: "6px 4px",
                    cursor: "pointer",
                    color: palette.textSecondary,
                    textTransform: "uppercase",
                    fontSize: 11,
                    fontWeight: 600,
                    letterSpacing: 0.5,
                }}
                onClick={() => setOpen((v) => !v)}
            >
                {open ? <CaretDownOutlined /> : <CaretRightOutlined />}
                <span>PROJECT: {project.slug}</span>
                <button
                    type="button"
                    aria-label={`Overview of ${project.slug}`}
                    onClick={(e) => {
                        e.stopPropagation();
                        onSelect({ kind: "overview", projectId: project.id });
                    }}
                    style={{
                        marginLeft: "auto",
                        background: "transparent",
                        border: "none",
                        color: palette.textSecondary,
                        cursor: "pointer",
                        fontSize: 11,
                        textTransform: "none",
                        fontWeight: 400,
                    }}
                >
                    overview
                </button>
            </header>

            {open && (
                <>
                    <SubHeader
                        label="Listeners"
                        count={listeners.length}
                        open={listenersOpen}
                        toggle={() => setListenersOpen((v) => !v)}
                    />
                    {listenersOpen &&
                        listeners.map((l) => (
                            <ListenerRow
                                key={l.id}
                                listener={l}
                                selected={
                                    selection?.kind === "listener" &&
                                    selection.listenerId === l.id
                                }
                                onSelect={() =>
                                    onSelect({
                                        kind: "listener",
                                        projectId: project.id,
                                        listenerId: l.id,
                                    })
                                }
                            />
                        ))}
                    {listenersOpen && listeners.length === 0 && (
                        <Muted indent={24}>No listeners yet.</Muted>
                    )}

                    <SubHeader
                        label="Hosts"
                        count={hosts.length}
                        open={hostsOpen}
                        toggle={() => setHostsOpen((v) => !v)}
                    />
                    {hostsOpen &&
                        hosts.map((h) => (
                            <HostRow
                                key={h.id}
                                host={h}
                                selected={
                                    selection?.kind === "host" && selection.hostId === h.id
                                }
                                onSelect={() =>
                                    onSelect({
                                        kind: "host",
                                        projectId: project.id,
                                        hostId: h.id,
                                    })
                                }
                            />
                        ))}
                    {hostsOpen && hosts.length === 0 && (
                        <Muted indent={24}>No hosts yet.</Muted>
                    )}

                    <div
                        role="button"
                        tabIndex={0}
                        onClick={() =>
                            onSelect({ kind: "dispatch", projectId: project.id })
                        }
                        onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === " ") {
                                onSelect({ kind: "dispatch", projectId: project.id });
                            }
                        }}
                        style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 8,
                            padding: "6px 12px 6px 24px",
                            color:
                                selection?.kind === "dispatch" &&
                                selection.projectId === project.id
                                    ? palette.accent
                                    : palette.textSecondary,
                            cursor: "pointer",
                            fontSize: 13,
                        }}
                    >
                        <ThunderboltOutlined />
                        <span>Dispatch</span>
                    </div>
                </>
            )}
        </div>
    );
}

function SubHeader({
    label,
    count,
    open,
    toggle,
}: {
    label: string;
    count: number;
    open: boolean;
    toggle: () => void;
}) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: 6,
                padding: "6px 12px 6px 16px",
                color: palette.textSecondary,
                fontSize: 11,
                fontWeight: 600,
                letterSpacing: 0.5,
                cursor: "pointer",
                textTransform: "uppercase",
            }}
            onClick={toggle}
        >
            {open ? <CaretDownOutlined /> : <CaretRightOutlined />}
            <span>{label}</span>
            <span style={{ marginLeft: "auto", fontWeight: 400 }}>{count}</span>
        </div>
    );
}

function Muted({ children, indent }: { children: React.ReactNode; indent: number }) {
    return (
        <div
            style={{
                paddingLeft: indent,
                paddingTop: 2,
                paddingBottom: 6,
                color: palette.textSecondary,
                fontSize: 12,
                fontStyle: "italic",
            }}
        >
            {children}
        </div>
    );
}
