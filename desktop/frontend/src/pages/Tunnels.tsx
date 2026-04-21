import { useCallback, useEffect, useState } from "react";
import {
    Alert,
    Button,
    Card,
    Form,
    Input,
    message,
    Modal,
    Select,
    Space,
    Table,
    Tag,
    Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";

import {
    CreateTunnel,
    ListSessions,
    ListTunnels,
} from "../../wailsjs/go/app/App";
import type { api } from "../../wailsjs/go/models";

const { Title, Text } = Typography;

type Mode = "pull" | "push" | "dynamic" | "internet";

interface CreateForm {
    mode: Mode;
    srcAddress: string;
    dstAddress: string;
}

const MODE_DESC: Record<Mode, string> = {
    pull: "Inbound: agent listens on dst, forwards to src on operator side",
    push: "Outbound: operator listens on src, agent dials dst on victim network",
    dynamic: "Agent runs a SOCKS5 server on a free port (request from agent)",
    internet: "Server runs SOCKS5 at src and proxies via the agent to dst",
};

export default function Tunnels() {
    const [sessions, setSessions] = useState<api.Session[]>([]);
    const [activeSession, setActiveSession] = useState<string>("");
    const [tunnels, setTunnels] = useState<api.TunnelInfo[]>([]);
    const [modalOpen, setModalOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    const [form] = Form.useForm<CreateForm>();
    const [messageApi, contextHolder] = message.useMessage();

    const loadSessions = useCallback(async () => {
        const s = await ListSessions();
        setSessions(s);
        if (!activeSession && s.length > 0) {
            setActiveSession(s[0].hash);
        }
    }, [activeSession]);

    const refreshTunnels = useCallback(async () => {
        if (!activeSession) return;
        try {
            setTunnels(await ListTunnels(activeSession));
        } catch (err) {
            messageApi.error(`refresh: ${String(err)}`);
        }
    }, [activeSession, messageApi]);

    useEffect(() => {
        loadSessions();
    }, [loadSessions]);

    useEffect(() => {
        refreshTunnels();
    }, [refreshTunnels]);

    async function handleCreate(v: CreateForm) {
        setBusy(true);
        try {
            await CreateTunnel(activeSession, v.mode, v.srcAddress, v.dstAddress);
            messageApi.success(`${v.mode} tunnel created`);
            setModalOpen(false);
            form.resetFields();
            refreshTunnels();
        } catch (err) {
            messageApi.error(`create: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    const columns: ColumnsType<api.TunnelInfo> = [
        {
            title: "Type",
            dataIndex: "type",
            key: "type",
            width: 120,
            render: (t: string) => <Tag color="blue">{t}</Tag>,
        },
        { title: "Address", dataIndex: "address", key: "addr" },
    ];

    return (
        <div>
            {contextHolder}
            <Space style={{ marginBottom: 12, width: "100%", justifyContent: "space-between" }}>
                <Title level={4} style={{ margin: 0 }}>
                    Tunnels
                </Title>
                <Space>
                    <Select
                        value={activeSession}
                        onChange={setActiveSession}
                        placeholder="Pick a termite session"
                        style={{ minWidth: 240 }}
                        options={sessions.map((s) => ({
                            label: `${s.alias || s.hash.slice(0, 12)} (${s.host}:${s.port})`,
                            value: s.hash,
                        }))}
                    />
                    <Button onClick={refreshTunnels}>Refresh</Button>
                    <Button
                        type="primary"
                        onClick={() => setModalOpen(true)}
                        disabled={!activeSession}
                    >
                        New Tunnel
                    </Button>
                </Space>
            </Space>

            {sessions.length === 0 ? (
                <Alert
                    type="info"
                    showIcon
                    message="No termite sessions. Tunnels require an encrypted (termite) session — open one or upgrade a plain shell first."
                />
            ) : (
                <Table
                    rowKey={(r) => `${r.type}:${r.address}`}
                    columns={columns}
                    dataSource={tunnels}
                    pagination={false}
                    size="middle"
                    locale={{ emptyText: "No active tunnels for this session." }}
                />
            )}

            <Modal
                title="New Tunnel"
                open={modalOpen}
                onOk={() => form.submit()}
                onCancel={() => setModalOpen(false)}
                okText="Create"
                confirmLoading={busy}
                destroyOnHidden
            >
                <Form
                    form={form}
                    layout="vertical"
                    initialValues={{ mode: "dynamic", srcAddress: "", dstAddress: "" }}
                    onFinish={handleCreate}
                >
                    <Form.Item name="mode" label="Mode" rules={[{ required: true }]}>
                        <Select
                            options={(["pull", "push", "dynamic", "internet"] as Mode[]).map((m) => ({
                                label: m,
                                value: m,
                            }))}
                        />
                    </Form.Item>
                    <Form.Item shouldUpdate noStyle>
                        {() => {
                            const mode = form.getFieldValue("mode") as Mode;
                            return (
                                <Card size="small" style={{ marginBottom: 12, background: "#fafafa" }}>
                                    <Text type="secondary">{MODE_DESC[mode]}</Text>
                                </Card>
                            );
                        }}
                    </Form.Item>
                    <Form.Item
                        name="srcAddress"
                        label="src_address"
                        extra="dynamic mode: ignored — server picks a free port"
                    >
                        <Input placeholder="0.0.0.0:1080" />
                    </Form.Item>
                    <Form.Item
                        name="dstAddress"
                        label="dst_address"
                        extra="dynamic: ignored. internet: target IP:port the SOCKS5 server proxies to."
                    >
                        <Input placeholder="127.0.0.1:80" />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
