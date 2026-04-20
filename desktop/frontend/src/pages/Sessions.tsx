import { useCallback, useEffect, useState } from "react";
import { Alert, Button, message, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

import { ListSessions } from "../../wailsjs/go/app/App";
import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";
import type { api } from "../../wailsjs/go/models";
import UpgradeModal from "./UpgradeModal";

dayjs.extend(relativeTime);

const { Title, Text } = Typography;

interface Props {
    onOpenTerminal: (s: api.Session) => void;
}

export default function Sessions({ onOpenTerminal }: Props) {
    const [sessions, setSessions] = useState<api.Session[]>([]);
    const [loading, setLoading] = useState(false);
    const [upgradeFor, setUpgradeFor] = useState<string>("");
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setSessions(await ListSessions());
        } catch (err) {
            messageApi.error(`refresh: ${String(err)}`);
        } finally {
            setLoading(false);
        }
    }, [messageApi]);

    useEffect(() => {
        refresh();
        EventsOn("notify:client_connected", () => {
            messageApi.success("New client connected");
            refresh();
        });
        EventsOn("notify:client_duplicated", () => {
            messageApi.warning("Duplicate client rejected");
        });
        return () => {
            EventsOff("notify:client_connected");
            EventsOff("notify:client_duplicated");
        };
    }, [refresh, messageApi]);

    const columns: ColumnsType<api.Session> = [
        {
            title: "Type",
            dataIndex: "tag",
            key: "tag",
            width: 100,
            render: (tag: string) => (
                <Tag color={tag === "termite" ? "blue" : "default"}>{tag || "shell"}</Tag>
            ),
        },
        { title: "Host", key: "host", render: (_, s) => `${s.host}:${s.port}` },
        {
            title: "Alias",
            dataIndex: "alias",
            key: "alias",
            render: (v: string) => v || <Text type="secondary">—</Text>,
        },
        {
            title: "User",
            dataIndex: "user",
            key: "user",
            render: (v: string) => (
                <Tag color={v === "root" ? "red" : undefined}>{v || "unknown"}</Tag>
            ),
        },
        { title: "OS", dataIndex: "os", key: "os" },
        {
            title: "Version",
            dataIndex: "version",
            key: "version",
            render: (v: string) => v || <Text type="secondary">—</Text>,
        },
        {
            title: "Online",
            dataIndex: "timestamp",
            key: "ts",
            render: (v: string) => dayjs(v).fromNow(),
        },
        {
            title: "Group",
            dataIndex: "group_dispatch",
            key: "group_dispatch",
            render: (v: boolean) => (v ? <Tag color="green">ON</Tag> : null),
        },
        {
            title: "",
            key: "open",
            width: 220,
            render: (_, s) => (
                <Space>
                    <Button type="link" onClick={() => onOpenTerminal(s)}>
                        Open Terminal
                    </Button>
                    {s.tag === "shell" && (
                        <Button type="link" onClick={() => setUpgradeFor(s.hash)}>
                            Upgrade
                        </Button>
                    )}
                </Space>
            ),
        },
    ];

    return (
        <div>
            {contextHolder}
            <Space style={{ marginBottom: 12, justifyContent: "space-between", width: "100%" }}>
                <Title level={4} style={{ margin: 0 }}>
                    Sessions ({sessions.length})
                </Title>
                <Button onClick={refresh} loading={loading}>
                    Refresh
                </Button>
            </Space>

            {sessions.length === 0 ? (
                <Alert
                    type="info"
                    showIcon
                    message="No sessions yet. Create a listener on the server and wait for clients to connect."
                />
            ) : (
                <Table
                    rowKey="hash"
                    columns={columns}
                    dataSource={sessions}
                    pagination={{ pageSize: 20 }}
                    size="middle"
                />
            )}

            <UpgradeModal
                open={!!upgradeFor}
                sessionHash={upgradeFor}
                onClose={() => setUpgradeFor("")}
            />
        </div>
    );
}
