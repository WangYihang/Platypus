import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Modal, Spin, message } from "antd";
import { ArrowLeftOutlined, DeleteOutlined } from "@ant-design/icons";
import { useNavigate, useParams } from "react-router-dom";

import Card from "../components/Card";
import DataList from "../components/DataList";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import { useCurrentProject } from "../layout/ProjectShell";
import { space } from "../layout/theme";
import { Listener, deleteListener, listListeners } from "../lib/api";
import { fromNow } from "../lib/time";

// ListenerDetailPage is /projects/:slug/listeners/:listenerId.
// Detail card + Stop action. Back button returns to the list.
export default function ListenerDetailPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { listenerId } = useParams<{ listenerId: string }>();
    const [listener, setListener] = useState<Listener | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        if (!listenerId) return;
        setLoading(true);
        try {
            const list = await listListeners(project.id);
            setListener(list.find((l) => l.id === listenerId) ?? null);
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [project.id, listenerId]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    function handleDelete() {
        if (!listener) return;
        Modal.confirm({
            title: `Stop listener ${listener.host}:${listener.port}?`,
            content:
                "Existing sessions stay alive, but no new connections will be accepted. The row is removed from storage.",
            okText: "Stop",
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await deleteListener(project.id, listener.id);
                    messageApi.success("Listener stopped");
                    navigate(`/projects/${project.slug}/listeners`);
                } catch (e) {
                    messageApi.error(`delete: ${String(e)}`);
                }
            },
        });
    }

    if (loading && !listener) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                <Spin />
            </div>
        );
    }
    if (error) {
        return (
            <div style={{ padding: space[5] }}>
                <Alert type="error" message={error} />
            </div>
        );
    }
    if (!listener) {
        return (
            <EmptyState
                title="Listener not found"
                description="It may have been stopped, or you may have lost access."
                fill
                action={
                    <Button onClick={() => navigate(`/projects/${project.slug}/listeners`)}>
                        Back to listeners
                    </Button>
                }
            />
        );
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title={
                    <span style={{ display: "flex", alignItems: "center", gap: space[3] }}>
                        <Button
                            size="small"
                            icon={<ArrowLeftOutlined />}
                            onClick={() => navigate(`/projects/${project.slug}/listeners`)}
                        />
                        <Mono size={22}>{`${listener.host}:${listener.port}`}</Mono>
                    </span>
                }
                subtitle={`listener · created ${fromNow(listener.created_at)}`}
                actions={
                    <Button danger icon={<DeleteOutlined />} onClick={handleDelete}>
                        Stop listener
                    </Button>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                <div style={{ maxWidth: 720 }}>
                    <Card header="Listener">
                        <DataList
                            items={[
                                {
                                    label: "host:port",
                                    value: <Mono>{`${listener.host}:${listener.port}`}</Mono>,
                                },
                                {
                                    label: "public ip",
                                    value: listener.public_ip ? (
                                        <Mono>{listener.public_ip}</Mono>
                                    ) : (
                                        "—"
                                    ),
                                },
                                {
                                    label: "shell",
                                    value: <Mono>{listener.shell_path || "default"}</Mono>,
                                },
                                {
                                    label: "created",
                                    value: fromNow(listener.created_at),
                                },
                                {
                                    label: "status",
                                    value: <StatusPill tone="success">listening</StatusPill>,
                                },
                            ]}
                        />
                    </Card>
                </div>
            </div>
        </div>
    );
}
