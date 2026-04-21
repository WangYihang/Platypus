import { useCallback, useEffect, useState } from "react";
import { ConfigProvider, Layout, Spin, Tabs, Typography, Button, Space, Tag } from "antd";

import Connect from "./pages/Connect";
import Sessions from "./pages/Sessions";
import Listeners from "./pages/Listeners";
import Files from "./pages/Files";
import Tunnels from "./pages/Tunnels";
import Terminal from "./pages/Terminal";
import { antTheme } from "./layout/theme";
import { ConnectionStatus, Disconnect } from "../wailsjs/go/app/App";
import { EventsOff, EventsOn } from "../wailsjs/runtime/runtime";
import type { api, app } from "../wailsjs/go/models";
import "./App.css";

const { Header, Content } = Layout;
const { Title, Text } = Typography;

interface TermTab {
    key: string;
    title: string;
    sessionHash: string;
}

function App() {
    const [status, setStatus] = useState<app.ConnectionStatus | null>(null);
    const [tabs, setTabs] = useState<TermTab[]>([]);
    const [activeKey, setActiveKey] = useState<string>("sessions");

    async function refresh() {
        try {
            setStatus(await ConnectionStatus());
        } catch {
            // ignore — Wails runtime not ready yet on cold start.
        }
    }

    useEffect(() => {
        refresh();
        EventsOn("app:connection_changed", () => refresh());
        return () => EventsOff("app:connection_changed");
    }, []);

    // Close all open terminal tabs whenever the user disconnects.
    useEffect(() => {
        if (status && !status.connected && tabs.length > 0) {
            setTabs([]);
            setActiveKey("sessions");
        }
    }, [status, tabs.length]);

    const openTerminal = useCallback((s: api.Session) => {
        const key = `term-${s.hash}`;
        setTabs((prev) =>
            prev.find((t) => t.key === key)
                ? prev
                : [...prev, { key, title: s.alias || s.hash.slice(0, 8), sessionHash: s.hash }]
        );
        setActiveKey(key);
    }, []);

    const closeTab = useCallback((key: string) => {
        setTabs((prev) => prev.filter((t) => t.key !== key));
        setActiveKey((cur) => (cur === key ? "sessions" : cur));
    }, []);

    if (status === null) {
        return (
            <ConfigProvider theme={antTheme}>
                <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                    <Spin size="large" />
                </div>
            </ConfigProvider>
        );
    }
    if (!status.connected) {
        return (
            <ConfigProvider theme={antTheme}>
                <Connect />
            </ConfigProvider>
        );
    }

    const items = [
        {
            key: "sessions",
            label: "Sessions",
            closable: false,
            children: <Sessions onOpenTerminal={openTerminal} />,
        },
        {
            key: "listeners",
            label: "Listeners",
            closable: false,
            children: <Listeners />,
        },
        {
            key: "files",
            label: "Files",
            closable: false,
            children: <Files />,
        },
        {
            key: "tunnels",
            label: "Tunnels",
            closable: false,
            children: <Tunnels />,
        },
        ...tabs.map((t) => ({
            key: t.key,
            label: t.title,
            closable: true,
            children: (
                <Terminal sessionHash={t.sessionHash} onClose={() => closeTab(t.key)} />
            ),
        })),
    ];

    return (
        <ConfigProvider theme={antTheme}>
            <Layout style={{ minHeight: "100vh" }}>
                <Header
                    style={{
                        background: "#1f1f1f",
                        padding: "0 24px",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                    }}
                >
                    <Title level={3} style={{ color: "#fff", margin: 0 }}>
                        Platypus Desktop
                    </Title>
                    <Space>
                        <Tag color="green">{status.profileName}</Tag>
                        <Text style={{ color: "#999" }}>{status.url}</Text>
                        <Button size="small" onClick={() => Disconnect()}>
                            Disconnect
                        </Button>
                    </Space>
                </Header>
                <Content style={{ padding: 16, height: "calc(100vh - 64px)" }}>
                    <Tabs
                        type="editable-card"
                        hideAdd
                        activeKey={activeKey}
                        onChange={setActiveKey}
                        onEdit={(key, action) => {
                            if (action === "remove") closeTab(key as string);
                        }}
                        items={items}
                        style={{ height: "100%" }}
                    />
                </Content>
            </Layout>
        </ConfigProvider>
    );
}

export default App;
