import { useEffect, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    message,
    Select,
    Space,
    Statistic,
    Typography,
} from "antd";
import { DownloadOutlined, UploadOutlined } from "@ant-design/icons";

import {
    DownloadFile,
    FileSize,
    ListSessions,
    PickFileToUpload,
    PickSaveLocation,
    UploadFile,
} from "../../wailsjs/go/app/App";
import type { api } from "../../wailsjs/go/models";

const { Title, Text } = Typography;

interface PathForm {
    sessionHash: string;
    remotePath: string;
}

export default function Files() {
    const [sessions, setSessions] = useState<api.Session[]>([]);
    const [size, setSize] = useState<number | null>(null);
    const [busy, setBusy] = useState<string>("");
    const [form] = Form.useForm<PathForm>();
    const [messageApi, contextHolder] = message.useMessage();

    useEffect(() => {
        ListSessions().then(setSessions);
    }, []);

    async function refreshSize() {
        const v = await form.validateFields();
        setBusy("size");
        try {
            const s = await FileSize(v.sessionHash, v.remotePath);
            setSize(s);
        } catch (err) {
            messageApi.error(`size: ${String(err)}`);
            setSize(null);
        } finally {
            setBusy("");
        }
    }

    async function download() {
        const v = await form.validateFields();
        const dst = await PickSaveLocation("Save to", basename(v.remotePath));
        if (!dst) return;
        setBusy("download");
        try {
            await DownloadFile(v.sessionHash, v.remotePath, dst);
            messageApi.success(`Saved to ${dst}`);
        } catch (err) {
            messageApi.error(`download: ${String(err)}`);
        } finally {
            setBusy("");
        }
    }

    async function upload() {
        const v = await form.validateFields();
        const src = await PickFileToUpload("Choose local file");
        if (!src) return;
        setBusy("upload");
        try {
            await UploadFile(v.sessionHash, v.remotePath, src);
            messageApi.success(`Uploaded ${src} → ${v.remotePath}`);
        } catch (err) {
            messageApi.error(`upload: ${String(err)}`);
        } finally {
            setBusy("");
        }
    }

    return (
        <div>
            {contextHolder}
            <Title level={4} style={{ marginTop: 0 }}>
                File transfer
            </Title>
            <Alert
                type="info"
                showIcon
                message="Path-based for now (no directory listing). Provide an exact remote path."
                style={{ marginBottom: 12 }}
            />

            <Form form={form} layout="vertical" style={{ maxWidth: 720 }}>
                <Form.Item
                    name="sessionHash"
                    label="Session"
                    rules={[{ required: true }]}
                >
                    <Select
                        showSearch
                        placeholder="Select session"
                        options={sessions.map((s) => ({
                            label: `${s.alias || s.hash.slice(0, 12)} (${s.host}:${s.port})`,
                            value: s.hash,
                        }))}
                    />
                </Form.Item>
                <Form.Item
                    name="remotePath"
                    label="Remote path"
                    rules={[{ required: true }]}
                >
                    <Input placeholder="/etc/hostname" />
                </Form.Item>
                <Space>
                    <Button onClick={refreshSize} loading={busy === "size"}>
                        Get Size
                    </Button>
                    <Button
                        icon={<DownloadOutlined />}
                        onClick={download}
                        loading={busy === "download"}
                    >
                        Download
                    </Button>
                    <Button
                        icon={<UploadOutlined />}
                        onClick={upload}
                        loading={busy === "upload"}
                        type="primary"
                    >
                        Upload
                    </Button>
                </Space>
            </Form>

            {size !== null && (
                <div style={{ marginTop: 16 }}>
                    <Statistic title="Size" value={`${size} bytes`} />
                    <Text type="secondary">≈ {humanize(size)}</Text>
                </div>
            )}
        </div>
    );
}

function basename(p: string): string {
    const i = p.lastIndexOf("/");
    return i >= 0 ? p.slice(i + 1) : p;
}

function humanize(n: number): string {
    const units = ["B", "KiB", "MiB", "GiB"];
    let v = n;
    let unit = 0;
    while (v >= 1024 && unit < units.length - 1) {
        v /= 1024;
        unit++;
    }
    return `${v.toFixed(unit === 0 ? 0 : 2)} ${units[unit]}`;
}
