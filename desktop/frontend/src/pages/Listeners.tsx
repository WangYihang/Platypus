import { useCallback, useEffect, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    InputNumber,
    message,
    Modal,
    Popover,
    Select,
    Space,
    Switch,
    Table,
    Tag,
    Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";

import {
    AvailableRaasLanguages,
    CreateListener,
    DeleteListener,
    GenerateRaasOneliner,
    ListListeners,
} from "../../wailsjs/go/app/App";
import type { api } from "../../wailsjs/go/models";

const { Title, Text, Paragraph } = Typography;

interface AddForm {
    host: string;
    port: number;
    encrypted: boolean;
}

export default function Listeners() {
    const [items, setItems] = useState<api.Listener[]>([]);
    const [loading, setLoading] = useState(false);
    const [modalOpen, setModalOpen] = useState(false);
    const [langs, setLangs] = useState<string[]>([]);
    const [form] = Form.useForm<AddForm>();
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setItems(await ListListeners());
        } catch (err) {
            messageApi.error(`refresh: ${String(err)}`);
        } finally {
            setLoading(false);
        }
    }, [messageApi]);

    useEffect(() => {
        refresh();
        AvailableRaasLanguages().then(setLangs);
    }, [refresh]);

    async function handleAdd(v: AddForm) {
        try {
            await CreateListener(v.host, v.port, v.encrypted);
            messageApi.success(`Listener on ${v.host}:${v.port} created`);
            setModalOpen(false);
            form.resetFields();
            refresh();
        } catch (err) {
            messageApi.error(`create: ${String(err)}`);
        }
    }

    function handleDelete(hash: string) {
        Modal.confirm({
            title: "Stop this listener?",
            content: "Existing sessions stay alive but no new connections will be accepted.",
            okText: "Stop",
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await DeleteListener(hash);
                    refresh();
                } catch (err) {
                    messageApi.error(`delete: ${String(err)}`);
                }
            },
        });
    }

    const columns: ColumnsType<api.Listener> = [
        {
            title: "Endpoint",
            key: "endpoint",
            render: (_, l) => (
                <Space>
                    <Text>{`${l.host}:${l.port}`}</Text>
                    {l.encrypted && <Tag color="blue">termite</Tag>}
                </Space>
            ),
        },
        {
            title: "Public IP",
            dataIndex: "public_ip",
            key: "publicIP",
            render: (v: string) => v || <Text type="secondary">—</Text>,
        },
        {
            title: "Sessions",
            dataIndex: "NumSessions",
            key: "n",
            render: (n: number) => <Tag color={n > 0 ? "green" : undefined}>{n}</Tag>,
        },
        {
            title: "RaaS oneliner",
            key: "raas",
            render: (_, l) => <RaasPopover listener={l} languages={langs} />,
        },
        {
            title: "",
            key: "actions",
            width: 80,
            render: (_, l) => (
                <Button danger type="link" onClick={() => handleDelete(l.hash)}>
                    Stop
                </Button>
            ),
        },
    ];

    return (
        <div>
            {contextHolder}
            <Space style={{ marginBottom: 12, justifyContent: "space-between", width: "100%" }}>
                <Title level={4} style={{ margin: 0 }}>
                    Listeners ({items.length})
                </Title>
                <Space>
                    <Button onClick={refresh} loading={loading}>
                        Refresh
                    </Button>
                    <Button type="primary" onClick={() => setModalOpen(true)}>
                        New Listener
                    </Button>
                </Space>
            </Space>

            {items.length === 0 ? (
                <Alert type="info" showIcon message="No listeners running. Create one to start accepting reverse shells." />
            ) : (
                <Table rowKey="hash" columns={columns} dataSource={items} pagination={false} size="middle" />
            )}

            <Modal
                title="New Listener"
                open={modalOpen}
                onOk={() => form.submit()}
                onCancel={() => setModalOpen(false)}
                okText="Create"
                destroyOnHidden
            >
                <Form
                    form={form}
                    layout="vertical"
                    initialValues={{ host: "0.0.0.0", port: 13337, encrypted: false }}
                    onFinish={handleAdd}
                >
                    <Form.Item name="host" label="Host" rules={[{ required: true }]}>
                        <Input placeholder="0.0.0.0" />
                    </Form.Item>
                    <Form.Item name="port" label="Port" rules={[{ required: true }]}>
                        <InputNumber min={1} max={65535} style={{ width: "100%" }} />
                    </Form.Item>
                    <Form.Item
                        name="encrypted"
                        label="Encrypted (Termite)"
                        valuePropName="checked"
                        extra="Use the Termite protocol (TLS + protobuf). Otherwise plain TCP reverse shell."
                    >
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}

function RaasPopover({ listener, languages }: { listener: api.Listener; languages: string[] }) {
    const [lang, setLang] = useState("bash");
    const [oneliner, setOneliner] = useState("");
    const [messageApi, contextHolder] = message.useMessage();

    async function regenerate(l: string) {
        const endpoint = `${listener.public_ip || listener.host}:${listener.port}`;
        setOneliner(await GenerateRaasOneliner(endpoint, l));
    }

    useEffect(() => {
        regenerate(lang);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [lang, listener.public_ip, listener.host, listener.port]);

    async function copy() {
        try {
            await navigator.clipboard.writeText(oneliner);
            messageApi.success("Copied");
        } catch {
            messageApi.error("Copy failed");
        }
    }

    const content = (
        <div style={{ width: 480 }}>
            {contextHolder}
            <Space style={{ marginBottom: 8 }}>
                <Select
                    size="small"
                    value={lang}
                    onChange={setLang}
                    options={languages.map((l) => ({ label: l, value: l }))}
                    style={{ width: 120 }}
                />
                <Button size="small" onClick={copy}>
                    Copy
                </Button>
            </Space>
            <Paragraph
                code
                style={{
                    whiteSpace: "pre-wrap",
                    wordBreak: "break-all",
                    maxHeight: 200,
                    overflow: "auto",
                }}
            >
                {oneliner || "—"}
            </Paragraph>
        </div>
    );

    return (
        <Popover content={content} trigger="click" placement="bottomLeft">
            <Button type="link">Show</Button>
        </Popover>
    );
}
