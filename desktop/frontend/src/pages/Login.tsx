import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, message, Space, Tabs, Typography } from "antd";

import { bootstrap, login } from "../lib/auth";
import { palette } from "../layout/theme";

const { Title, Text, Paragraph } = Typography;

interface LoginFormValues {
    url: string;
    username: string;
    password: string;
}

interface BootstrapFormValues {
    url: string;
    secret: string;
    username: string;
    password: string;
}

interface Props {
    onLoggedIn: () => void;
    initialURL?: string;
}

// Login is the post-connect gate for both web and desktop modes. Users
// either log in with username+password (normal flow) or bootstrap the
// first admin using the server secret printed at startup (one-shot
// setup).
//
// Chose a tabbed view over a separate setup route because admins type
// the bootstrap secret once ever — they shouldn't have to remember a
// special URL for it.
export default function Login({ onLoggedIn, initialURL }: Props) {
    const [busy, setBusy] = useState(false);
    const [messageApi, contextHolder] = message.useMessage();

    // Default URL guess: the existing connection (desktop) or the current
    // origin with the server's typical port (web).
    const defaultURL =
        initialURL ||
        (typeof window !== "undefined"
            ? window.location.origin.replace(/:\d+$/, ":7331")
            : "");

    useEffect(() => {
        // Put cursor on username if a URL is already known.
        // noop — focus handled by autoFocus attribute below.
    }, []);

    async function doLogin(v: LoginFormValues) {
        setBusy(true);
        try {
            await login(v.url, v.username, v.password);
            onLoggedIn();
        } catch (err) {
            messageApi.error(`login: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    async function doBootstrap(v: BootstrapFormValues) {
        setBusy(true);
        try {
            await bootstrap(v.url, v.secret, v.username, v.password);
            messageApi.success("Admin created — welcome to Platypus");
            onLoggedIn();
        } catch (err) {
            messageApi.error(`bootstrap: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <div
            style={{
                minHeight: "100vh",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                padding: 24,
                background: palette.main,
                color: palette.textPrimary,
            }}
        >
            {contextHolder}
            <Card
                style={{
                    width: 440,
                    maxWidth: "100%",
                    background: palette.sidebar,
                    border: `1px solid ${palette.border}`,
                }}
                styles={{ body: { padding: 24 } }}
            >
                <Space direction="vertical" size="large" style={{ width: "100%" }}>
                    <div>
                        <Title level={3} style={{ margin: 0, color: palette.textPrimary }}>
                            Platypus
                        </Title>
                        <Text style={{ color: palette.textSecondary }}>
                            Log in to your server or bootstrap the first admin.
                        </Text>
                    </div>

                    <Tabs
                        defaultActiveKey="login"
                        items={[
                            {
                                key: "login",
                                label: "Log in",
                                children: (
                                    <Form
                                        layout="vertical"
                                        initialValues={{ url: defaultURL }}
                                        onFinish={doLogin}
                                    >
                                        <Form.Item
                                            name="url"
                                            label="Server URL"
                                            rules={[
                                                { required: true, message: "Required" },
                                                { type: "url", message: "Must be a valid URL" },
                                            ]}
                                        >
                                            <Input placeholder="http://127.0.0.1:7331" />
                                        </Form.Item>
                                        <Form.Item
                                            name="username"
                                            label="Username"
                                            rules={[{ required: true }]}
                                        >
                                            <Input autoFocus placeholder="admin" />
                                        </Form.Item>
                                        <Form.Item
                                            name="password"
                                            label="Password"
                                            rules={[{ required: true }]}
                                        >
                                            <Input.Password />
                                        </Form.Item>
                                        <Button
                                            type="primary"
                                            htmlType="submit"
                                            block
                                            loading={busy}
                                        >
                                            Log in
                                        </Button>
                                    </Form>
                                ),
                            },
                            {
                                key: "bootstrap",
                                label: "First-time setup",
                                children: (
                                    <Form
                                        layout="vertical"
                                        initialValues={{ url: defaultURL, username: "admin" }}
                                        onFinish={doBootstrap}
                                    >
                                        <Alert
                                            type="info"
                                            showIcon
                                            message="One-shot admin setup"
                                            description={
                                                <Paragraph style={{ margin: 0 }}>
                                                    Use the <Text code>API bootstrap secret</Text>{" "}
                                                    printed on server startup. After the first
                                                    admin exists this tab stops working.
                                                </Paragraph>
                                            }
                                            style={{ marginBottom: 16 }}
                                        />
                                        <Form.Item
                                            name="url"
                                            label="Server URL"
                                            rules={[{ required: true }]}
                                        >
                                            <Input />
                                        </Form.Item>
                                        <Form.Item
                                            name="secret"
                                            label="Server secret"
                                            rules={[{ required: true }]}
                                        >
                                            <Input.Password />
                                        </Form.Item>
                                        <Form.Item
                                            name="username"
                                            label="Admin username"
                                            rules={[{ required: true }]}
                                        >
                                            <Input />
                                        </Form.Item>
                                        <Form.Item
                                            name="password"
                                            label="Admin password"
                                            rules={[
                                                { required: true, min: 8, message: "Min 8 chars" },
                                            ]}
                                        >
                                            <Input.Password />
                                        </Form.Item>
                                        <Button
                                            type="primary"
                                            htmlType="submit"
                                            block
                                            loading={busy}
                                        >
                                            Create admin
                                        </Button>
                                    </Form>
                                ),
                            },
                        ]}
                    />
                </Space>
            </Card>
        </div>
    );
}
