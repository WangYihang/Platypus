import { useEffect, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    Layout,
    List,
    message,
    Modal,
    Space,
    Tag,
    Typography,
} from "antd";
import {
    AddProfile,
    Connect as ConnectProfile,
    ConnectionStatus,
    Disconnect,
    ListProfiles,
    RemoveProfile,
} from "../../wailsjs/go/app/App";
import { app, profile } from "../../wailsjs/go/models";

const { Header, Content } = Layout;
const { Title, Text } = Typography;

interface AddFormValues {
    name: string;
    url: string;
    secret: string;
}

export default function Connect() {
    const [profiles, setProfiles] = useState<profile.Profile[]>([]);
    const [status, setStatus] = useState<app.ConnectionStatus>(
        new app.ConnectionStatus({ connected: false, profileName: "", url: "" })
    );
    const [modalOpen, setModalOpen] = useState(false);
    const [busy, setBusy] = useState<string>(""); // current operation target
    const [form] = Form.useForm<AddFormValues>();
    const [messageApi, contextHolder] = message.useMessage();

    async function refresh() {
        try {
            const [list, st] = await Promise.all([ListProfiles(), ConnectionStatus()]);
            setProfiles(list);
            setStatus(st);
        } catch (err) {
            messageApi.error(`refresh: ${String(err)}`);
        }
    }

    useEffect(() => {
        refresh();
    }, []);

    async function handleAdd(v: AddFormValues) {
        try {
            await AddProfile(v.name, v.url, v.secret);
            setModalOpen(false);
            form.resetFields();
            messageApi.success(`Added ${v.name}`);
            refresh();
        } catch (err) {
            messageApi.error(`add: ${String(err)}`);
        }
    }

    async function handleConnect(name: string) {
        setBusy(`connect:${name}`);
        try {
            await ConnectProfile(name);
            messageApi.success(`Connected to ${name}`);
            refresh();
        } catch (err) {
            messageApi.error(`connect: ${String(err)}`);
        } finally {
            setBusy("");
        }
    }

    async function handleDisconnect() {
        setBusy("disconnect");
        try {
            await Disconnect();
            refresh();
        } finally {
            setBusy("");
        }
    }

    async function handleRemove(name: string) {
        Modal.confirm({
            title: `Remove profile "${name}"?`,
            content: "Saved URL and the secret in the OS keychain will be deleted.",
            okText: "Remove",
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await RemoveProfile(name);
                    messageApi.success(`Removed ${name}`);
                    refresh();
                } catch (err) {
                    messageApi.error(`remove: ${String(err)}`);
                }
            },
        });
    }

    return (
        <Layout style={{ minHeight: "100vh" }}>
            {contextHolder}
            <Header style={{ background: "#1f1f1f", padding: "0 24px" }}>
                <Title level={3} style={{ color: "#fff", margin: "12px 0" }}>
                    Platypus Desktop
                </Title>
            </Header>

            <Content style={{ padding: 24 }}>
                {status.connected ? (
                    <Alert
                        type="success"
                        showIcon
                        message={
                            <Space>
                                <span>Connected to</span>
                                <Tag color="green">{status.profileName}</Tag>
                                <Text type="secondary">{status.url}</Text>
                            </Space>
                        }
                        action={
                            <Button
                                size="small"
                                onClick={handleDisconnect}
                                loading={busy === "disconnect"}
                            >
                                Disconnect
                            </Button>
                        }
                        style={{ marginBottom: 16 }}
                    />
                ) : (
                    <Alert
                        type="info"
                        showIcon
                        message="Not connected. Pick a profile below or add a new one."
                        style={{ marginBottom: 16 }}
                    />
                )}

                <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 12 }}>
                    <Title level={4} style={{ margin: 0 }}>
                        Server Profiles
                    </Title>
                    <Button type="primary" onClick={() => setModalOpen(true)}>
                        Add Profile
                    </Button>
                </div>

                <List
                    bordered
                    dataSource={profiles}
                    locale={{ emptyText: "No profiles yet — click Add Profile to start." }}
                    renderItem={(p) => {
                        const isCurrent = status.connected && status.profileName === p.name;
                        return (
                            <List.Item
                                actions={[
                                    isCurrent ? (
                                        <Tag color="green" key="active">
                                            Active
                                        </Tag>
                                    ) : (
                                        <Button
                                            key="connect"
                                            type="link"
                                            loading={busy === `connect:${p.name}`}
                                            onClick={() => handleConnect(p.name)}
                                        >
                                            Connect
                                        </Button>
                                    ),
                                    <Button
                                        key="remove"
                                        type="link"
                                        danger
                                        onClick={() => handleRemove(p.name)}
                                    >
                                        Remove
                                    </Button>,
                                ]}
                            >
                                <List.Item.Meta title={p.name} description={p.url} />
                            </List.Item>
                        );
                    }}
                />
            </Content>

            <Modal
                title="Add Server Profile"
                open={modalOpen}
                onCancel={() => setModalOpen(false)}
                onOk={() => form.submit()}
                okText="Add"
                destroyOnHidden
            >
                <Form form={form} layout="vertical" onFinish={handleAdd}>
                    <Form.Item
                        name="name"
                        label="Profile name"
                        rules={[{ required: true, message: "Required" }]}
                    >
                        <Input placeholder="e.g. local, prod-eu" autoFocus />
                    </Form.Item>
                    <Form.Item
                        name="url"
                        label="Server URL"
                        rules={[
                            { required: true, message: "Required" },
                            { type: "url", message: "Must be a valid http(s) URL" },
                        ]}
                    >
                        <Input placeholder="http://127.0.0.1:7331" />
                    </Form.Item>
                    <Form.Item
                        name="secret"
                        label="Server secret"
                        rules={[{ required: true, message: "Required" }]}
                        extra="Stored in the OS keychain. Token is fetched on Connect."
                    >
                        <Input.Password />
                    </Form.Item>
                </Form>
            </Modal>
        </Layout>
    );
}
