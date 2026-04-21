import { useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    InputNumber,
    Space,
    Table,
    Tag,
    Typography,
    message,
} from "antd";
import type { ColumnsType } from "antd/es/table";

import MainHeader from "../layout/MainHeader";
import { palette } from "../layout/theme";
import { DispatchResult, dispatchCommand } from "../lib/api";

interface Props {
    projectID: string;
    projectName: string;
}

// DispatchPanel is the in-line "run a command on every flagged live
// session" view. Slack-like in placement: selecting Dispatch in the
// sidebar opens it as the main-panel content, no modal. Results stay
// visible so operators can compare runs without reopening a dialog.
export default function DispatchPanel({ projectID, projectName }: Props) {
    const [form] = Form.useForm<{ command: string; timeout: number }>();
    const [busy, setBusy] = useState(false);
    const [results, setResults] = useState<DispatchResult[] | null>(null);
    const [messageApi, contextHolder] = message.useMessage();

    async function run() {
        const v = await form.validateFields();
        setBusy(true);
        try {
            setResults(await dispatchCommand(projectID, v.command, v.timeout));
        } catch (e) {
            messageApi.error(`dispatch: ${String(e)}`);
        } finally {
            setBusy(false);
        }
    }

    const columns: ColumnsType<DispatchResult> = [
        {
            title: "Session",
            dataIndex: "session_hash",
            render: (v: string) => <code style={{ fontSize: 11 }}>{v.slice(0, 16)}…</code>,
            width: 180,
        },
        {
            title: "Host",
            dataIndex: "host_id",
            render: (v: string) => <code style={{ fontSize: 11 }}>{v.slice(0, 8)}…</code>,
            width: 120,
        },
        {
            title: "Output",
            render: (_, r) =>
                r.error ? (
                    <Tag color="red">{r.error}</Tag>
                ) : (
                    <pre
                        style={{
                            margin: 0,
                            whiteSpace: "pre-wrap",
                            fontSize: 12,
                            color: palette.textPrimary,
                        }}
                    >
                        {r.output || <Typography.Text type="secondary">(empty)</Typography.Text>}
                    </pre>
                ),
        },
    ];

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <MainHeader
                title="Dispatch"
                subtitle={`Run a command on every flagged live session in ${projectName}`}
            />
            <div style={{ flex: 1, overflow: "auto", padding: 20 }}>
                <Alert
                    type="info"
                    showIcon
                    message="Sessions are targeted by their group_dispatch flag"
                    description="Flip a session's group_dispatch flag to include it in dispatches. The server only runs the command on sessions that are (a) currently live and (b) flagged."
                    style={{ marginBottom: 16 }}
                />
                <Form
                    form={form}
                    layout="vertical"
                    initialValues={{ command: "id", timeout: 3 }}
                    onFinish={run}
                    style={{ maxWidth: 720 }}
                >
                    <Form.Item
                        name="command"
                        label="Command"
                        rules={[{ required: true, message: "command is required" }]}
                    >
                        <Input.TextArea autoSize={{ minRows: 1, maxRows: 6 }} placeholder="id" />
                    </Form.Item>
                    <Form.Item
                        name="timeout"
                        label="Per-session timeout (seconds)"
                        extra="Each session gets its own timeout — slow boxes don't block the rest."
                    >
                        <InputNumber min={1} max={120} />
                    </Form.Item>
                    <Space>
                        <Button type="primary" onClick={run} loading={busy}>
                            Run
                        </Button>
                        <Button onClick={() => setResults(null)} disabled={!results}>
                            Clear results
                        </Button>
                    </Space>
                </Form>

                {results !== null && (
                    <div style={{ marginTop: 24 }}>
                        <Typography.Title
                            level={5}
                            style={{ color: palette.textPrimary, marginTop: 0 }}
                        >
                            Results ({results.length})
                        </Typography.Title>
                        <Table
                            rowKey="session_hash"
                            columns={columns}
                            dataSource={results}
                            pagination={false}
                            size="small"
                            locale={{ emptyText: "No flagged live sessions." }}
                        />
                    </div>
                )}
            </div>
        </div>
    );
}
