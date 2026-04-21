import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Button, message, Space, Switch, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

import { ListSessions, SetGroupDispatch } from "../../wailsjs/go/app/App";
import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";
import type { api } from "../../wailsjs/go/models";
import DispatchModal from "./DispatchModal";

dayjs.extend(relativeTime);

const { Title, Text } = Typography;

interface Props {
    onOpenTerminal: (s: api.Session) => void;
}

export default function Sessions({ onOpenTerminal }: Props) {
    const [sessions, setSessions] = useState<api.Session[]>([]);
    const [loading, setLoading] = useState(false);
    const [dispatchOpen, setDispatchOpen] = useState(false);
    // Optimistic local override so the Switch toggle feels instant — we
    // overlay this on whatever ListSessions returned.
    const [pendingFlags, setPendingFlags] = useState<Record<string, boolean>>({});
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

    const effectiveFlag = useCallback(
        (s: api.Session): boolean =>
            s.hash in pendingFlags ? pendingFlags[s.hash] : s.group_dispatch,
        [pendingFlags]
    );

    const toggleFlag = useCallback(
        async (hash: string, enabled: boolean) => {
            setPendingFlags((p) => ({ ...p, [hash]: enabled }));
            try {
                await SetGroupDispatch(hash, enabled);
            } catch (err) {
                messageApi.error(`toggle: ${String(err)}`);
                // Roll back the optimistic flip.
                setPendingFlags((p) => {
                    const { [hash]: _, ...rest } = p;
                    return rest;
                });
            }
        },
        [messageApi]
    );

    const flaggedCount = useMemo(
        () => sessions.filter(effectiveFlag).length,
        [sessions, effectiveFlag]
    );

    const columns: ColumnsType<api.Session> = [
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
            width: 90,
            render: (_v: boolean, s) => (
                <Switch
                    size="small"
                    checked={effectiveFlag(s)}
                    onChange={(checked) => toggleFlag(s.hash, checked)}
                />
            ),
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
                <Space>
                    {flaggedCount > 0 && <Tag color="green">{flaggedCount} flagged</Tag>}
                    <Button
                        onClick={() => setDispatchOpen(true)}
                        disabled={flaggedCount === 0}
                    >
                        Dispatch Command
                    </Button>
                    <Button onClick={refresh} loading={loading}>
                        Refresh
                    </Button>
                </Space>
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

            <DispatchModal
                open={dispatchOpen}
                flaggedCount={flaggedCount}
                onClose={() => setDispatchOpen(false)}
            />
        </div>
    );
}
