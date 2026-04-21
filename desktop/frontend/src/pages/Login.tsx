import { useState } from "react";
import { Button, Form, Input, message, Tabs } from "antd";

import Card from "../components/Card";
import { bootstrap, login } from "../lib/auth";
import { font, palette, space } from "../layout/theme";

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

// Login is the auth gate for both web and desktop modes. Two flows in
// one card via a tab control: standard username+password login, and
// the one-shot bootstrap flow that creates the first admin from the
// API secret printed on server startup.
export default function Login({ onLoggedIn, initialURL }: Props) {
    const [busy, setBusy] = useState(false);
    const [messageApi, contextHolder] = message.useMessage();

    const defaultURL =
        initialURL ||
        (typeof window !== "undefined"
            ? window.location.origin.replace(/:\d+$/, ":7331")
            : "");

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
                padding: space[6],
                background: palette.main,
                color: palette.textPrimary,
            }}
        >
            {contextHolder}
            <div style={{ width: 440, maxWidth: "100%" }}>
                <div style={{ marginBottom: space[6], textAlign: "left" }}>
                    <h1
                        style={{
                            margin: 0,
                            color: palette.textPrimary,
                            fontFamily: font.sans,
                            fontWeight: 600,
                            fontSize: 28,
                            lineHeight: 1.2,
                            letterSpacing: -0.2,
                        }}
                    >
                        Platypus
                    </h1>
                    <p
                        style={{
                            margin: `${space[2]}px 0 0`,
                            color: palette.textSecondary,
                            fontSize: 14,
                            lineHeight: 1.5,
                        }}
                    >
                        Log in to your server, or bootstrap the first admin from the
                        startup secret.
                    </p>
                </div>

                <Card padding={6}>
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
                                        <p
                                            style={{
                                                margin: `0 0 ${space[4]}px`,
                                                color: palette.textSecondary,
                                                fontSize: 13,
                                                lineHeight: 1.5,
                                            }}
                                        >
                                            Use the{" "}
                                            <span style={{ fontFamily: font.mono, fontSize: 12 }}>
                                                API bootstrap secret
                                            </span>{" "}
                                            printed on server startup. After the first admin
                                            exists this tab stops working.
                                        </p>
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
                </Card>
            </div>
        </div>
    );
}
