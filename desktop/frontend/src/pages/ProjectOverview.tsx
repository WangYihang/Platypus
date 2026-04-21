import { useEffect, useState } from "react";
import { Button, Card, Col, Row, Statistic, Typography } from "antd";
import { TeamOutlined } from "@ant-design/icons";

import MainHeader from "../layout/MainHeader";
import { palette } from "../layout/theme";
import { Host, Listener, Project, listHosts, listListeners } from "../lib/api";
import { isOnline } from "../lib/time";

interface Props {
    project: Project;
    onOpenMembers?: () => void;
}

// ProjectOverview is the zero-selection dashboard for a project: three
// big numbers (listeners, hosts, online hosts) plus "what's fresh"
// callouts. Deliberately sparse — it's the empty-state you see before
// you've picked a host; the real work happens in HostView.
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

    return (
        <div
            style={{ display: "flex", flexDirection: "column", height: "100%" }}
        >
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
            <div style={{ padding: 20, overflow: "auto" }}>
                {error && (
                    <Typography.Paragraph type="danger">{error}</Typography.Paragraph>
                )}
                <Row gutter={16}>
                    <Col span={8}>
                        <Card style={statCard}>
                            <Statistic
                                title="Listeners"
                                value={listeners?.length ?? "—"}
                            />
                        </Card>
                    </Col>
                    <Col span={8}>
                        <Card style={statCard}>
                            <Statistic title="Hosts" value={hosts?.length ?? "—"} />
                        </Card>
                    </Col>
                    <Col span={8}>
                        <Card style={statCard}>
                            <Statistic
                                title="Online now"
                                value={onlineCount}
                                valueStyle={{ color: palette.success }}
                            />
                        </Card>
                    </Col>
                </Row>

                <Typography.Title
                    level={5}
                    style={{ color: palette.textPrimary, marginTop: 24 }}
                >
                    Getting started
                </Typography.Title>
                <Typography.Paragraph style={{ color: palette.textSecondary }}>
                    {listeners && listeners.length === 0
                        ? "No listeners yet — use the sidebar to create one, then run platypus-agent on a host."
                        : "Pick a host in the sidebar to open its terminal, files, and tunnels."}
                </Typography.Paragraph>
            </div>
        </div>
    );
}

const statCard: React.CSSProperties = {
    background: palette.sidebar,
    border: `1px solid ${palette.border}`,
};
