import { useEffect, useState } from "react";
import { Button, Typography } from "antd";
import { TeamOutlined } from "@ant-design/icons";

import Card from "../components/Card";
import MetricCard from "../components/MetricCard";
import MainHeader from "../layout/MainHeader";
import { palette, space } from "../layout/theme";
import { Host, Listener, Project, listHosts, listListeners } from "../lib/api";
import { isOnline } from "../lib/time";

interface Props {
    project: Project;
    onOpenMembers?: () => void;
}

// ProjectOverview is the zero-selection dashboard for a project: three
// metric tiles (listeners, hosts, online hosts) plus a getting-started
// callout. Sparse by design — the real work happens once a host or
// listener is picked from the sidebar.
export default function ProjectOverview({ project, onOpenMembers }: Props) {
    const [listeners, setListeners] = useState<Listener[] | null>(null);
    const [hosts, setHosts] = useState<Host[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const [l, h] = await Promise.all([
                    listListeners(project.id),
                    listHosts(project.id),
                ]);
                if (!cancelled) {
                    setListeners(l);
                    setHosts(h);
                }
            } catch (e) {
                if (!cancelled) setError(String(e));
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [project.id]);

    const onlineCount = hosts?.filter((h) => isOnline(h.last_seen_at)).length ?? 0;
    const noListenersYet = listeners !== null && listeners.length === 0;

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <MainHeader
                title={project.name}
                subtitle={`${project.slug} · overview`}
                actions={
                    onOpenMembers && (
                        <Button
                            size="small"
                            icon={<TeamOutlined />}
                            onClick={onOpenMembers}
                        >
                            Members
                        </Button>
                    )
                }
            />
            <div
                style={{
                    flex: 1,
                    overflow: "auto",
                    padding: space[6],
                }}
            >
                <div style={{ maxWidth: 1200, margin: "0 auto" }}>
                    {error && (
                        <Typography.Paragraph type="danger">{error}</Typography.Paragraph>
                    )}
                    <div
                        style={{
                            display: "grid",
                            gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
                            gap: space[3],
                        }}
                    >
                        <MetricCard
                            label="Listeners"
                            value={listeners?.length ?? "—"}
                        />
                        <MetricCard label="Hosts" value={hosts?.length ?? "—"} />
                        <MetricCard
                            label="Online now"
                            value={onlineCount}
                            accent={onlineCount > 0 ? "success" : "default"}
                            hint={
                                hosts && hosts.length > 0
                                    ? `${onlineCount} of ${hosts.length} hosts reachable`
                                    : undefined
                            }
                        />
                    </div>

                    <div style={{ marginTop: space[6] }}>
                        <Card header="Getting started">
                            <p
                                style={{
                                    margin: 0,
                                    color: palette.textSecondary,
                                    fontSize: 13,
                                    lineHeight: 1.55,
                                }}
                            >
                                {noListenersYet
                                    ? "No listeners yet — use the sidebar to create one, then run platypus-agent on a host to register it with this project."
                                    : "Pick a host in the sidebar to open its terminal, files, and tunnels. Use Dispatch to run a command across every flagged session at once."}
                            </p>
                        </Card>
                    </div>
                </div>
            </div>
        </div>
    );
}
