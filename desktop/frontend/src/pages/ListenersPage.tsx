import { useCallback, useEffect, useMemo, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    InputNumber,
    Modal,
    Spin,
    Table,
    message,
} from "antd";
import {
    GatewayOutlined,
    PlusOutlined,
    ReloadOutlined,
    SearchOutlined,
} from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import { useNavigate } from "react-router-dom";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { Listener, createListener, deleteListener, listListeners } from "../lib/api";
import { fromNow } from "../lib/time";

// ListenersPage is the cross-listener list at /projects/:slug/listeners.
// Always-visible "+ New listener" button in PageHeader actions —
// solves the original "no entry point to create a listener" problem.
export default function ListenersPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const [listeners, setListeners] = useState<Listener[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [createOpen, setCreateOpen] = useState(false);
    const [createForm] = Form.useForm<{ host: string; port: number }>();
    const [query, setQuery] = useState("");
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setListeners(await listListeners(project.id));
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [project.id]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function handleCreate() {
        const v = await createForm.validateFields();
        try {
            const l = await createListener(project.id, v.host, v.port);
            messageApi.success(`Listener ${l.host}:${l.port} created`);
            setCreateOpen(false);
            createForm.resetFields();
            await refresh();
            navigate(`/projects/${project.slug}/listeners/${l.id}`);
        } catch (e) {
            messageApi.error(`create: ${String(e)}`);
        }
    }

    function handleDelete(l: Listener) {
        Modal.confirm({
            title: `Stop listener ${l.host}:${l.port}?`,
            content:
                "Existing sessions stay alive, but no new connections will be accepted. The row is removed from storage.",
            okText: "Stop",
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await deleteListener(project.id, l.id);
                    messageApi.success("Listener stopped");
                    refresh();
                } catch (e) {
                    messageApi.error(`delete: ${String(e)}`);
                }
            },
        });
    }

    const filtered = useMemo(() => {
        if (!listeners) return null;
        const q = query.trim().toLowerCase();
        if (!q) return listeners;
        return listeners.filter((l) =>
            `${l.host}:${l.port} ${l.public_ip ?? ""}`.toLowerCase().includes(q),
        );
    }, [listeners, query]);

    const columns: ColumnsType<Listener> = [
        {
            title: "Endpoint",
            dataIndex: "id",
            render: (_, l) => <Mono size={13}>{`${l.host}:${l.port}`}</Mono>,
        },
        {
            title: "Public IP",
            dataIndex: "public_ip",
            render: (v?: string) =>
                v ? <Mono>{v}</Mono> : <span style={{ color: palette.textMuted }}>—</span>,
        },
        {
            title: "Created",
            dataIndex: "created_at",
            render: (v: string) => fromNow(v),
            width: 160,
        },
        {
            title: "Status",
            render: () => <StatusPill tone="success">listening</StatusPill>,
            width: 120,
        },
        {
            title: "",
            width: 80,
            render: (_, l) => (
                <Button
                    size="small"
                    danger
                    type="link"
                    onClick={(e) => {
                        e.stopPropagation();
                        handleDelete(l);
                    }}
                >
                    Stop
                </Button>
            ),
        },
    ];

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title="Listeners"
                subtitle={
                    listeners === null ? "Loading…" : `${listeners.length} total`
                }
                actions={
                    <>
                        <Button
                            size="small"
                            icon={<ReloadOutlined />}
                            loading={loading}
                            onClick={refresh}
                        >
                            Refresh
                        </Button>
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            onClick={() => setCreateOpen(true)}
                        >
                            New listener
                        </Button>
                    </>
                }
            />
            <Toolbar
                left={
                    <Input
                        prefix={<SearchOutlined style={{ color: palette.textMuted }} />}
                        placeholder="Search host:port or public IP"
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        allowClear
                        style={{ maxWidth: 360 }}
                    />
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <Alert
                        type="error"
                        message={error}
                        style={{ marginBottom: space[4] }}
                    />
                )}
                {!listeners && (
                    <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                        <Spin />
                    </div>
                )}
                {listeners && listeners.length === 0 && (
                    <EmptyState
                        icon={<GatewayOutlined />}
                        title="No listeners yet"
                        description="Create a listener to start accepting agent connections. Each listener binds to a host:port and stays running until you stop it."
                        action={
                            <Button
                                type="primary"
                                icon={<PlusOutlined />}
                                onClick={() => setCreateOpen(true)}
                            >
                                New listener
                            </Button>
                        }
                    />
                )}
                {filtered && filtered.length === 0 && listeners && listeners.length > 0 && (
                    <EmptyState
                        title="No matches"
                        description={`No listener matches "${query}".`}
                    />
                )}
                {filtered && filtered.length > 0 && (
                    <Card padding={0}>
                        <Table
                            rowKey="id"
                            columns={columns}
                            dataSource={filtered}
                            pagination={false}
                            size="small"
                            bordered={false}
                            onRow={(l) => ({
                                onClick: () =>
                                    navigate(
                                        `/projects/${project.slug}/listeners/${l.id}`,
                                    ),
                                style: { cursor: "pointer" },
                            })}
                        />
                    </Card>
                )}
            </div>

            <Modal
                title="New listener"
                open={createOpen}
                onOk={handleCreate}
                onCancel={() => setCreateOpen(false)}
                okText="Create"
                destroyOnHidden
            >
                <Form
                    form={createForm}
                    layout="vertical"
                    initialValues={{ host: "0.0.0.0", port: 13337 }}
                >
                    <Form.Item name="host" label="Host" rules={[{ required: true }]}>
                        <Input placeholder="0.0.0.0" />
                    </Form.Item>
                    <Form.Item name="port" label="Port" rules={[{ required: true }]}>
                        <InputNumber min={1} max={65535} style={{ width: "100%" }} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
