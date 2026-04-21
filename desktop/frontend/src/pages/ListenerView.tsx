import { useCallback, useEffect, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    InputNumber,
    message,
    Modal,
    Space,
    Spin,
    Table,
} from "antd";
import { DeleteOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import Card from "../components/Card";
import DataList from "../components/DataList";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import StatusPill from "../components/StatusPill";
import PageHeader from "../components/PageHeader";
import { space } from "../layout/theme";
import { Listener, createListener, deleteListener, listListeners } from "../lib/api";
import { fromNow } from "../lib/time";

interface Props {
    projectID: string;
    listenerID?: string;
    onSelectListener?: (id: string) => void;
}

// ListenerView handles both the per-listener detail view and the list
// view (when no specific id is selected). Same chrome (header + actions)
// either way; the body switches between a detail card and a Vercel-style
// dense table.
export default function ListenerView({
    projectID,
    listenerID,
    onSelectListener,
}: Props) {
    const [listeners, setListeners] = useState<Listener[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [createOpen, setCreateOpen] = useState(false);
    const [createForm] = Form.useForm<{ host: string; port: number }>();
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setListeners(await listListeners(projectID));
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [projectID]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function handleCreate() {
        const v = await createForm.validateFields();
        try {
            const l = await createListener(projectID, v.host, v.port);
            messageApi.success(`Listener ${l.host}:${l.port} created`);
            setCreateOpen(false);
            createForm.resetFields();
            refresh();
            onSelectListener?.(l.id);
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
                    await deleteListener(projectID, l.id);
                    messageApi.success("Listener stopped");
                    refresh();
                } catch (e) {
                    messageApi.error(`delete: ${String(e)}`);
                }
            },
        });
    }

    const selected = listeners?.find((l) => l.id === listenerID);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title={
                    selected ? (
                        <Mono size={16}>{`${selected.host}:${selected.port}`}</Mono>
                    ) : (
                        "Listeners"
                    )
                }
                subtitle={
                    selected
                        ? `listener · created ${fromNow(selected.created_at)}`
                        : `${listeners?.length ?? 0} total`
                }
                actions={
                    <Space size={space[2]}>
                        <Button
                            size="small"
                            icon={<ReloadOutlined />}
                            loading={loading}
                            onClick={refresh}
                        >
                            Refresh
                        </Button>
                        <Button
                            size="small"
                            type="primary"
                            icon={<PlusOutlined />}
                            onClick={() => setCreateOpen(true)}
                        >
                            New listener
                        </Button>
                    </Space>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[6] }}>
                <div style={{ maxWidth: 1200, margin: "0 auto" }}>
                    {error && (
                        <Alert type="error" message={error} style={{ marginBottom: space[4] }} />
                    )}
                    {!listeners && (
                        <div style={{ display: "flex", justifyContent: "center", padding: 40 }}>
                            <Spin />
                        </div>
                    )}
                    {selected ? (
                        <ListenerDetail
                            listener={selected}
                            onDelete={() => handleDelete(selected)}
                        />
                    ) : (
                        listeners && (
                            <ListenerTable
                                listeners={listeners}
                                onPick={onSelectListener}
                                onDelete={handleDelete}
                                onNew={() => setCreateOpen(true)}
                            />
                        )
                    )}
                </div>
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

function ListenerDetail({
    listener,
    onDelete,
}: {
    listener: Listener;
    onDelete: () => void;
}) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4], maxWidth: 720 }}>
            <Card header="Listener">
                <DataList
                    items={[
                        {
                            label: "host:port",
                            value: <Mono>{`${listener.host}:${listener.port}`}</Mono>,
                        },
                        {
                            label: "public ip",
                            value: listener.public_ip ? <Mono>{listener.public_ip}</Mono> : "—",
                        },
                        { label: "shell", value: <Mono>{listener.shell_path || "default"}</Mono> },
                        { label: "created", value: fromNow(listener.created_at) },
                        {
                            label: "status",
                            value: <StatusPill tone="success">listening</StatusPill>,
                        },
                    ]}
                />
            </Card>
            <div>
                <Button danger icon={<DeleteOutlined />} onClick={onDelete}>
                    Stop this listener
                </Button>
            </div>
        </div>
    );
}

function ListenerTable({
    listeners,
    onPick,
    onDelete,
    onNew,
}: {
    listeners: Listener[];
    onPick?: (id: string) => void;
    onDelete: (l: Listener) => void;
    onNew: () => void;
}) {
    if (listeners.length === 0) {
        return (
            <EmptyState
                title="No listeners yet"
                description="Create a listener to start accepting agent connections."
                action={
                    <Button type="primary" icon={<PlusOutlined />} onClick={onNew}>
                        New listener
                    </Button>
                }
            />
        );
    }

    const columns: ColumnsType<Listener> = [
        {
            title: "Endpoint",
            dataIndex: "id",
            render: (_, l) => (
                <button
                    type="button"
                    onClick={() => onPick?.(l.id)}
                    style={{
                        background: "transparent",
                        border: "none",
                        padding: 0,
                        cursor: onPick ? "pointer" : "default",
                        textAlign: "left",
                        color: "inherit",
                    }}
                >
                    <Mono size={13}>{`${l.host}:${l.port}`}</Mono>
                </button>
            ),
        },
        {
            title: "Public IP",
            dataIndex: "public_ip",
            render: (v?: string) => (v ? <Mono>{v}</Mono> : "—"),
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
                    onClick={() => onDelete(l)}
                >
                    Stop
                </Button>
            ),
        },
    ];

    return (
        <Card padding={0}>
            <Table
                rowKey="id"
                columns={columns}
                dataSource={listeners}
                pagination={false}
                size="small"
                bordered={false}
            />
        </Card>
    );
}
