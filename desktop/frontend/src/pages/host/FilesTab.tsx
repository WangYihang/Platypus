import { useState } from "react";
import { Alert, Button, Form, Input, Space, Statistic, Typography, message } from "antd";
import { DownloadOutlined, UploadOutlined } from "@ant-design/icons";

import {
    DownloadFile,
    FileSize,
    PickFileToUpload,
    PickSaveLocation,
    UploadFile,
} from "../../../wailsjs/go/app/App";
import { basename, humanize } from "../../lib/format";
import { palette } from "../../layout/theme";

interface Props {
    sessionHash: string;
}

// FilesTab is the per-session file-transfer panel embedded in HostView.
// Unlike the legacy pages/Files.tsx which carried its own session-picker
// <Select>, this variant takes `sessionHash` as a prop — the picker now
// lives in the HostView's parent (chip row above the sub-tabs).
//
// Implementation re-uses the Wails bindings that already support both
// desktop (native file dialogs) and web (HTMLInputElement-based) modes
// via the platform shim in src/platform/App.web.ts.
export default function FilesTab({ sessionHash }: Props) {
    const [size, setSize] = useState<number | null>(null);
    const [busy, setBusy] = useState<string>("");
    const [form] = Form.useForm<{ remotePath: string }>();
    const [messageApi, contextHolder] = message.useMessage();

    async function refreshSize() {
        const v = await form.validateFields();
        setBusy("size");
        try {
            setSize(await FileSize(sessionHash, v.remotePath));
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
            await DownloadFile(sessionHash, v.remotePath, dst);
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
            await UploadFile(sessionHash, v.remotePath, src);
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
            <Alert
                type="info"
                showIcon
                message="Path-based for now (no directory listing)"
                description="Provide an exact remote path, then Get Size / Download / Upload."
                style={{ marginBottom: 12 }}
            />

            <Form form={form} layout="vertical" style={{ maxWidth: 720 }}>
                <Form.Item
                    name="remotePath"
                    label="Remote path"
                    rules={[{ required: true }]}
                >
                    <Input placeholder="/etc/hostname" autoFocus />
                </Form.Item>
                <Space>
                    <Button onClick={refreshSize} loading={busy === "size"}>
                        Get size
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
                    <Typography.Text style={{ color: palette.textSecondary }}>
                        ≈ {humanize(size)}
                    </Typography.Text>
                </div>
            )}
        </div>
    );
}
