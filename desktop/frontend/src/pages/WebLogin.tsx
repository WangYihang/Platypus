import { useState } from "react";
import {
    Alert,
    Button,
    Card,
    Form,
    Input,
    Layout,
    message,
    Space,
    Typography,
} from "antd";

import { webLogin } from "../platform/App.web";

const { Title, Text, Paragraph } = Typography;

interface FormValues {
    url: string;
    secret: string;
}

// WebLogin is the entry point for the standalone web UI. Unlike the
// desktop Connect page there's no profile CRUD — the web UI points at
// one server at a time and persists the url+token in localStorage.
export default function WebLogin({ onLoggedIn }: { onLoggedIn: () => void }) {
    const [busy, setBusy] = useState(false);
    const [form] = Form.useForm<FormValues>();
    const [messageApi, contextHolder] = message.useMessage();

    async function submit(v: FormValues) {
        setBusy(true);
        try {
            await webLogin(v.url, v.secret);
            messageApi.success("Connected");
            onLoggedIn();
        } catch (err) {
            messageApi.error(`login: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <Layout style={{ minHeight: "100vh", background: "#f5f5f5" }}>
            {contextHolder}
            <Layout.Content
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    padding: 24,
                }}
            >
                <Card style={{ width: 480, maxWidth: "100%" }}>
                    <Space direction="vertical" size="large" style={{ width: "100%" }}>
                        <div>
                            <Title level={3} style={{ margin: 0 }}>
                                Platypus Web UI
                            </Title>
                            <Text type="secondary">
                                Connect to a platypus-server instance to manage sessions,
                                listeners, and tunnels.
                            </Text>
                        </div>

                        <Form
                            form={form}
                            layout="vertical"
                            initialValues={{ url: window.location.origin.replace(/:\d+$/, ":7331") }}
                            onFinish={submit}
                        >
                            <Form.Item
                                name="url"
                                label="Server URL"
                                rules={[
                                    { required: true, message: "Required" },
                                    { type: "url", message: "Must be a valid http(s) URL" },
                                ]}
                            >
                                <Input placeholder="http://127.0.0.1:7331" autoFocus />
                            </Form.Item>

                            <Form.Item
                                name="secret"
                                label="API secret"
                                rules={[{ required: true, message: "Required" }]}
                                extra={
                                    <span>
                                        Printed on server startup as{" "}
                                        <Text code>API secret: …</Text>
                                    </span>
                                }
                            >
                                <Input.Password />
                            </Form.Item>

                            <Button type="primary" htmlType="submit" block loading={busy}>
                                Connect
                            </Button>
                        </Form>

                        <Alert
                            type="info"
                            showIcon
                            message="Stored locally"
                            description={
                                <Paragraph style={{ margin: 0 }}>
                                    The URL and bearer token are cached in{" "}
                                    <Text code>localStorage</Text>. Click Disconnect in the
                                    header to clear.
                                </Paragraph>
                            }
                        />
                    </Space>
                </Card>
            </Layout.Content>
        </Layout>
    );
}
