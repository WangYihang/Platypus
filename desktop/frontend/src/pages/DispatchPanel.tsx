import { useState } from "react";
import {
    Button,
    Form,
    Input,
    InputNumber,
    Space,
    Table,
    Typography,
    message,
} from "antd";
import type { ColumnsType } from "antd/es/table";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import StatusPill from "../components/StatusPill";
import MainHeader from "../layout/MainHeader";
import { font, palette, space } from "../layout/theme";
import { DispatchResult, dispatchCommand } from "../lib/api";

interface Props {
    projectID: string;
    projectName: string;
}

// DispatchPanel is the in-line "run a command on every flagged live
// session" view. Selecting Dispatch in the sidebar opens it as the
// main-panel content (no modal) so results stay visible across runs.
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
            render: (v: string) => <Mono>{v.slice(0, 16)}…</Mono>,
            width: 180,
        },
        {
            title: "Host",
            dataIndex: "host_id",
            render: (v: string) => <Mono>{v.slice(0, 8)}…</Mono>,
            width: 120,
        },
        {
            title: "Output",
            render: (_, r) =>
                r.error ? (
                    <StatusPill tone="danger">{r.error}</StatusPill>
                ) : (
                    <pre
                        style={{
                            margin: 0,
                            whiteSpace: "pre-wrap",
                            fontFamily: font.mono,
                            fontSize: 12,
                            color: palette.textPrimary,
                        }}
                    >
                        {r.output || (
                            <Typography.Text type="secondary">(empty)</Typography.Text>
                        )}
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
            <div style={{ flex: 1, overflow: "auto", padding: space[6] }}>
                <div
                    style={{
                        maxWidth: 1200,
                        margin: "0 auto",
                        display: "flex",
                        flexDirection: "column",
                        gap: space[4],
                    }}
                >
                    <Card header="Run command" style={{ maxWidth: 720 }}>
                        <p
                            style={{
                                margin: `0 0 ${space[4]}px`,
                                color: palette.textSecondary,
                                fontSize: 13,
                                lineHeight: 1.5,
                            }}
                        >
                            Sessions are targeted by their{" "}
                            <Mono>group_dispatch</Mono> flag. Flip a session's flag from its
                            HostView Sessions tab to include it. Only sessions that are{" "}
                            <em>live</em> and <em>flagged</em> will receive the command.
                        </p>
                        <Form
                            form={form}
                            layout="vertical"
                            initialValues={{ command: "id", timeout: 3 }}
                            onFinish={run}
                        >
                            <Form.Item
                                name="command"
                                label="Command"
                                rules={[{ required: true, message: "command is required" }]}
                            >
                                <Input.TextArea
                                    autoSize={{ minRows: 1, maxRows: 6 }}
                                    placeholder="id"
                                    style={{ fontFamily: font.mono }}
                                />
                            </Form.Item>
                            <Form.Item
                                name="timeout"
                                label="Per-session timeout (seconds)"
                                extra="Each session gets its own timeout — slow boxes don't block the rest."
                            >
                                <InputNumber min={1} max={120} />
                            </Form.Item>
                            <Space size={space[2]}>
                                <Button type="primary" onClick={run} loading={busy}>
                                    Run
                                </Button>
                                <Button onClick={() => setResults(null)} disabled={!results}>
                                    Clear results
                                </Button>
                            </Space>
                        </Form>
                    </Card>

                    {results !== null && (
                        <Card
                            header={
                                <span>
                                    Results <Mono color={palette.textSecondary}>({results.length})</Mono>
                                </span>
                            }
                            padding={0}
                        >
                            {results.length === 0 ? (
                                <EmptyState
                                    title="No flagged live sessions"
                                    description="Flip a session's group_dispatch flag from its host's Sessions tab to include it here."
                                />
                            ) : (
                                <Table
                                    rowKey="session_hash"
                                    columns={columns}
                                    dataSource={results}
                                    pagination={false}
                                    size="small"
                                    bordered={false}
                                />
                            )}
                        </Card>
                    )}
                </div>
            </div>
        </div>
    );
}
