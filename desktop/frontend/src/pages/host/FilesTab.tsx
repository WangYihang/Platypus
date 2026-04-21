import { useState } from "react";
import { Button, Form, Input, Space, message } from "antd";
import { DownloadOutlined, UploadOutlined } from "@ant-design/icons";

import Card from "../../components/Card";
import DataList from "../../components/DataList";
import Mono from "../../components/Mono";
import {
    DownloadFile,
    FileSize,
    PickFileToUpload,
    PickSaveLocation,
    UploadFile,
} from "../../../wailsjs/go/app/App";
import { basename, humanize } from "../../lib/format";
import { palette, space } from "../../layout/theme";

interface Props {
    sessionHash: string;
}

// FilesTab is the per-session file-transfer panel embedded in HostView.
// Path-based for now (no directory listing): the user types an exact
// remote path, then Get Size / Download / Upload act on it.
export default function FilesTab({ sessionHash }: Props) {
    const [size, setSize] = useState<number | null>(null);
    const [lastPath, setLastPath] = useState<string | null>(null);
    const [busy, setBusy] = useState<string>("");
    const [form] = Form.useForm<{ remotePath: string }>();
    const [messageApi, contextHolder] = message.useMessage();

    async function refreshSize() {
        const v = await form.validateFields();
        setBusy("size");
        try {
            const n = await FileSize(sessionHash, v.remotePath);
            setSize(n);
            setLastPath(v.remotePath);
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
        <div style={{ maxWidth: 720, display: "flex", flexDirection: "column", gap: space[4] }}>
            {contextHolder}

            <Card header="Transfer">
                <p
                    style={{
                        margin: `0 0 ${space[4]}px`,
                        color: palette.textSecondary,
                        fontSize: 13,
                        lineHeight: 1.5,
                    }}
                >
                    Path-based for now — provide an exact remote path, then Get Size /
                    Download / Upload.
                </p>
                <Form form={form} layout="vertical">
                    <Form.Item
                        name="remotePath"
                        label="Remote path"
                        rules={[{ required: true }]}
                    >
                        <Input placeholder="/etc/hostname" autoFocus />
                    </Form.Item>
                    <Space size={space[2]}>
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
            </Card>

            {size !== null && lastPath && (
                <Card header="Size">
                    <DataList
                        items={[
                            { label: "path", value: <Mono>{lastPath}</Mono> },
                            { label: "bytes", value: <Mono>{size}</Mono> },
                            { label: "human", value: humanize(size) },
                        ]}
                    />
                </Card>
            )}
        </div>
    );
}
