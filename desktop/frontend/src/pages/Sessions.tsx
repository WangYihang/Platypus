import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Layout, message, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

import {
    ConnectionStatus,
    Disconnect,
    ListSessions,
} from "../../wailsjs/go/app/App";
import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";
import type { api, app } from "../../wailsjs/go/models";

dayjs.extend(relativeTime);

const { Header, Content } = Layout;
const { Title, Text } = Typography;

export default function Sessions() {
    const [sessions, setSessions] = useState<api.Session[]>([]);
    const [status, setStatus] = useState<app.ConnectionStatus | null>(null);
    const [loading, setLoading] = useState(false);
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const [st, list] = await Promise.all([ConnectionStatus(), ListSessions()]);
            setStatus(st);
            setSessions(list);
        } catch (err) {
            messageApi.error(`refresh: ${String(err)}`);
        } finally {
            setLoading(false);
        }
    }, [messageApi]);

    useEffect(() => {
        refresh();

        const onConnected = (payload: unknown) => {
            messageApi.success(
                `New client connected${typeof payload === "object" ? "" : ""}`
            );
            refresh();
        };
        const onDuplicated = () => {
            messageApi.warning("Duplicate client rejected");
        };

        EventsOn("notify:client_connected", onConnected);
        EventsOn("notify:client_duplicated", onDuplicated);
        return () => {
            EventsOff("notify:client_connected");
            EventsOff("notify:client_duplicated");
        };
    }, [refresh, messageApi]);

    async function handleDisconnect() {
        await Disconnect();
    }

    const columns: ColumnsType<api.Session> = [
        {
            title: "Type",
            dataIndex: "Tag",
            key: "tag",
            width: 100,
            render: (tag: string) => (
                <Tag color={tag === "termite" ? "blue" : "default"}>{tag || "shell"}</Tag>
            ),
        },
        {
            title: "Host",
            key: "host",
            render: (_, s) => `${s.host}:${s.port}`,
        },
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
        {
            title: "OS",
            dataIndex: "os",
            key: "os",
        },
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
    ];

    return (
        <Layout style={{ minHeight: "100vh" }}>
            {contextHolder}
            <Header
                style={{
                    background: "#1f1f1f",
                    padding: "0 24px",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                }}
            >
                <Title level={3} style={{ color: "#fff", margin: 0 }}>
                    Platypus Desktop
                </Title>
                {status?.connected && (
                    <Space>
                        <Tag color="green">{status.profileName}</Tag>
                        <Text style={{ color: "#999" }}>{status.url}</Text>
                        <Button size="small" onClick={handleDisconnect}>
                            Disconnect
                        </Button>
                    </Space>
                )}
            </Header>

            <Content style={{ padding: 24 }}>
                <div
                    style={{
                        display: "flex",
                        justifyContent: "space-between",
                        marginBottom: 12,
                    }}
                >
                    <Title level={4} style={{ margin: 0 }}>
                        Sessions
                    </Title>
                    <Button onClick={refresh} loading={loading}>
                        Refresh
                    </Button>
                </div>

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
            </Content>
        </Layout>
    );
}
