import { useCallback, useEffect, useState } from "react";
import {
    Alert,
    Button,
    Descriptions,
    Form,
    Input,
    InputNumber,
    message,
    Modal,
    Space,
    Spin,
} from "antd";
import { DeleteOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";

import MainHeader from "../layout/MainHeader";
import { palette } from "../layout/theme";
import { Listener, createListener, deleteListener, listListeners } from "../lib/api";
import { fromNow } from "../lib/time";

interface Props {
    projectID: string;
    listenerID?: string; // optional — when null we show the project's listener list
    onSelectListener?: (id: string) => void;
}

// ListenerView handles both the per-listener detail page and the list
// view (when no specific id is selected). The latter is reachable from
// the "overview" path if someone wants a full-width table instead of
// the sidebar rows; for now the sidebar also links straight to a
// specific listener id.
export default function ListenerView({ projectID, listenerID, onSelectListener }: Props) {
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

    async function handleDelete(l: Listener) {
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
            <MainHeader
                title={
                    selected ? `${selected.host}:${selected.port}` : "Listeners"
                }
                subtitle={
                    selected
                        ? `listener · ${fromNow(selected.created_at)} ago`
                        : `${listeners?.length ?? 0} total`
                }
                actions={
                    <Space>
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
            <div style={{ flex: 1, overflow: "auto", padding: 20 }}>
                {error && <Alert type="error" message={error} style={{ marginBottom: 16 }} />}
                {!listeners && (
                    <div style={{ display: "flex", justifyContent: "center", padding: 40 }}>
                        <Spin />
                    </div>
                )}
                {selected ? (
                    <ListenerDetail listener={selected} onDelete={() => handleDelete(selected)} />
                ) : (
                    listeners && <ListenerTable listeners={listeners} onPick={onSelectListener} onDelete={handleDelete} />
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

function ListenerDetail({
    listener,
    onDelete,
}: {
    listener: Listener;
    onDelete: () => void;
}) {
    return (
        <>
            <Descriptions
                size="small"
                column={1}
                bordered
                styles={{ label: { width: 180, color: palette.textSecondary } }}
            >
                <Descriptions.Item label="host:port">
                    {listener.host}:{listener.port}
                </Descriptions.Item>
                <Descriptions.Item label="public ip">
                    {listener.public_ip || "—"}
                </Descriptions.Item>
                <Descriptions.Item label="shell">
                    {listener.shell_path || "default"}
                </Descriptions.Item>
                <Descriptions.Item label="created">
                    {fromNow(listener.created_at)}
                </Descriptions.Item>
            </Descriptions>
            <div style={{ marginTop: 16 }}>
                <Button danger icon={<DeleteOutlined />} onClick={onDelete}>
                    Stop this listener
                </Button>
            </div>
        </>
    );
}

function ListenerTable({
    listeners,
    onPick,
    onDelete,
}: {
    listeners: Listener[];
    onPick?: (id: string) => void;
    onDelete: (l: Listener) => void;
}) {
    if (listeners.length === 0) {
        return (
            <Alert
                type="info"
                showIcon
                message="No listeners yet. Click New listener to create one."
            />
        );
    }
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {listeners.map((l) => (
                <div
                    key={l.id}
                    style={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        padding: "10px 14px",
                        background: palette.sidebar,
                        border: `1px solid ${palette.border}`,
                        borderRadius: 6,
                    }}
                >
                    <button
                        type="button"
                        onClick={() => onPick?.(l.id)}
                        style={{
                            background: "transparent",
                            border: "none",
                            color: palette.textPrimary,
                            cursor: onPick ? "pointer" : "default",
                            padding: 0,
                            fontSize: 14,
                        }}
                    >
                        {l.host}:{l.port}
                    </button>
                    <Space>
                        <span style={{ color: palette.textSecondary, fontSize: 12 }}>
                            {fromNow(l.created_at)}
                        </span>
                        <Button
                            size="small"
                            danger
                            type="link"
                            onClick={() => onDelete(l)}
                        >
                            Stop
                        </Button>
                    </Space>
                </div>
            ))}
        </div>
    );
}
