import { useCallback, useEffect, useState } from "react";
import {
    Alert,
    Button,
    Card,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Table,
    Tag,
    Typography,
    message,
} from "antd";
import type { ColumnsType } from "antd/es/table";

import { CreateTunnel, ListTunnels } from "../../../wailsjs/go/app/App";
import type { api } from "../../../wailsjs/go/models";

const { Title, Text } = Typography;

type Mode = "pull" | "push" | "dynamic" | "internet";

const MODE_DESC: Record<Mode, string> = {
    pull: "Inbound: agent listens on dst, forwards to src on operator side",
    push: "Outbound: operator listens on src, agent dials dst on its own network",
    dynamic: "Agent runs a SOCKS5 server on a free port (request from agent)",
    internet: "Server runs SOCKS5 at src and proxies via the agent to dst",
};

interface Props {
    sessionHash: string;
}

// TunnelsTab is the per-session tunnel manager embedded in HostView.
// Extracted from the legacy pages/Tunnels.tsx with the session-picker
// dropdown removed — sessionHash now flows in as a prop from the
// HostView chip row.
export default function TunnelsTab({ sessionHash }: Props) {
    const [tunnels, setTunnels] = useState<api.TunnelInfo[]>([]);
    const [modalOpen, setModalOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    const [form] = Form.useForm<{ mode: Mode; srcAddress: string; dstAddress: string }>();
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        try {
            setTunnels(await ListTunnels(sessionHash));
        } catch (err) {
            messageApi.error(`refresh: ${String(err)}`);
        }
    }, [sessionHash, messageApi]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function handleCreate() {
        const v = await form.validateFields();
        setBusy(true);
        try {
            await CreateTunnel(sessionHash, v.mode, v.srcAddress, v.dstAddress);
            messageApi.success(`${v.mode} tunnel created`);
            setModalOpen(false);
            form.resetFields();
            refresh();
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
                <Title level={5} style={{ margin: 0 }}>
                    Active tunnels
                </Title>
                <Space>
                    <Button onClick={refresh}>Refresh</Button>
                    <Button type="primary" onClick={() => setModalOpen(true)}>
                        New tunnel
                    </Button>
                </Space>
            </Space>

            <Table
                rowKey={(r) => `${r.type}:${r.address}`}
                columns={columns}
                dataSource={tunnels}
                pagination={false}
                size="middle"
                locale={{ emptyText: "No active tunnels for this session." }}
            />

            <Modal
                title="New tunnel"
                open={modalOpen}
                onOk={handleCreate}
                onCancel={() => setModalOpen(false)}
                okText="Create"
                confirmLoading={busy}
                destroyOnHidden
            >
                <Form
                    form={form}
                    layout="vertical"
                    initialValues={{ mode: "dynamic", srcAddress: "", dstAddress: "" }}
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
                                <Card size="small" style={{ marginBottom: 12 }}>
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

            {tunnels.length === 0 && (
                <Alert
                    type="info"
                    showIcon
                    style={{ marginTop: 16 }}
                    message="No tunnels yet — use New tunnel to open one on this session."
                />
            )}
        </div>
    );
}
