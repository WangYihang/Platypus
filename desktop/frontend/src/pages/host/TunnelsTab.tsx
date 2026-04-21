import { useCallback, useEffect, useState } from "react";
import {
    Button,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Table,
    message,
} from "antd";
import { PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import StatusPill from "../../components/StatusPill";
import { CreateTunnel, ListTunnels } from "../../../wailsjs/go/app/App";
import type { api } from "../../../wailsjs/go/models";
import { palette, space } from "../../layout/theme";

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
// sessionHash flows in as a prop from the HostView chip row.
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
            render: (t: string) => <StatusPill tone="info">{t}</StatusPill>,
        },
        {
            title: "Address",
            dataIndex: "address",
            key: "addr",
            render: (a: string) => <Mono>{a}</Mono>,
        },
    ];

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            {contextHolder}
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                }}
            >
                <h3
                    style={{
                        margin: 0,
                        color: palette.textPrimary,
                        fontSize: 14,
                        fontWeight: 600,
                    }}
                >
                    Active tunnels
                </h3>
                <Space size={space[2]}>
                    <Button icon={<ReloadOutlined />} size="small" onClick={refresh}>
                        Refresh
                    </Button>
                    <Button
                        icon={<PlusOutlined />}
                        size="small"
                        type="primary"
                        onClick={() => setModalOpen(true)}
                    >
                        New tunnel
                    </Button>
                </Space>
            </div>

            <Card padding={0}>
                <Table
                    rowKey={(r) => `${r.type}:${r.address}`}
                    columns={columns}
                    dataSource={tunnels}
                    pagination={false}
                    size="small"
                    bordered={false}
                    locale={{
                        emptyText: (
                            <EmptyState
                                title="No active tunnels"
                                description="Open a tunnel on this session via New tunnel."
                            />
                        ),
                    }}
                />
            </Card>

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
                                <p
                                    style={{
                                        margin: `0 0 ${space[4]}px`,
                                        color: palette.textSecondary,
                                        fontSize: 12,
                                        lineHeight: 1.5,
                                    }}
                                >
                                    {MODE_DESC[mode]}
                                </p>
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
