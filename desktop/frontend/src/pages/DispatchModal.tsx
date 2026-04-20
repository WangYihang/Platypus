import { useState } from "react";
import {
    Alert,
    Button,
    Empty,
    Form,
    Input,
    InputNumber,
    message,
    Modal,
    Space,
    Table,
    Tag,
    Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";

import { DispatchCommand } from "../../wailsjs/go/app/App";
import type { api } from "../../wailsjs/go/models";

const { Text } = Typography;

interface FormValues {
    command: string;
    timeout: number;
}

interface Props {
    open: boolean;
    flaggedCount: number;
    onClose: () => void;
}

export default function DispatchModal({ open, flaggedCount, onClose }: Props) {
    const [form] = Form.useForm<FormValues>();
    const [busy, setBusy] = useState(false);
    const [results, setResults] = useState<api.DispatchResult[] | null>(null);
    const [messageApi, contextHolder] = message.useMessage();

    async function run(v: FormValues) {
        setBusy(true);
        try {
            const r = await DispatchCommand(v.command, v.timeout);
            setResults(r ?? []);
        } catch (err) {
            messageApi.error(`dispatch: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    function handleClose() {
        form.resetFields();
        setResults(null);
        onClose();
    }

    const columns: ColumnsType<api.DispatchResult> = [
        {
            title: "Session",
            dataIndex: "session_hash",
            key: "session_hash",
            width: 220,
            render: (h: string) => <Text code>{h.slice(0, 16)}…</Text>,
        },
        {
            title: "Output",
            key: "output",
            render: (_, r) =>
                r.error ? (
                    <Tag color="red">{r.error}</Tag>
                ) : (
                    <pre style={{ margin: 0, whiteSpace: "pre-wrap", fontSize: 12 }}>
                        {r.output || <Text type="secondary">(empty)</Text>}
                    </pre>
                ),
        },
    ];

    return (
        <Modal
            title="Dispatch Command"
            open={open}
            onCancel={handleClose}
            width={860}
            footer={[
                <Button key="close" onClick={handleClose}>
                    Close
                </Button>,
                <Button
                    key="run"
                    type="primary"
                    loading={busy}
                    disabled={flaggedCount === 0}
                    onClick={() => form.submit()}
                >
                    Run
                </Button>,
            ]}
        >
            {contextHolder}
            {flaggedCount === 0 ? (
                <Alert
                    type="warning"
                    showIcon
                    message="No sessions are flagged for dispatch."
                    description="Toggle the Group switch on at least one session in the Sessions table, then come back."
                />
            ) : (
                <Alert
                    type="info"
                    showIcon
                    message={`${flaggedCount} session(s) will receive this command.`}
                    style={{ marginBottom: 12 }}
                />
            )}
            <Form
                form={form}
                layout="vertical"
                initialValues={{ command: "id", timeout: 3 }}
                onFinish={run}
                disabled={flaggedCount === 0}
            >
                <Form.Item
                    name="command"
                    label="Command"
                    rules={[{ required: true, message: "command is required" }]}
                >
                    <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} placeholder="id" />
                </Form.Item>
                <Form.Item name="timeout" label="Per-session timeout (s)">
                    <InputNumber min={1} max={120} />
                </Form.Item>
            </Form>

            {results !== null && (
                <Space direction="vertical" style={{ width: "100%", marginTop: 8 }}>
                    <Text strong>Results ({results.length})</Text>
                    {results.length === 0 ? (
                        <Empty description="No flagged sessions responded." />
                    ) : (
                        <Table
                            rowKey={(r) => r.session_hash}
                            columns={columns}
                            dataSource={results}
                            pagination={false}
                            size="small"
                        />
                    )}
                </Space>
            )}
        </Modal>
    );
}
