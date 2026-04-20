import { useEffect, useState } from "react";
import { Alert, Button, message, Modal, Progress, Select, Space, Typography } from "antd";

import { ListListeners, UpgradeToTermite } from "../../wailsjs/go/app/App";
import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";
import type { api } from "../../wailsjs/go/models";

const { Text } = Typography;

interface Props {
    open: boolean;
    sessionHash: string;
    onClose: () => void;
}

interface Progresses {
    compile: number;   // 0-100, -1 = error
    compress: number;
    upload: number;
}

const initialProgress: Progresses = { compile: 0, compress: 0, upload: 0 };

export default function UpgradeModal({ open, sessionHash, onClose }: Props) {
    const [encryptedListeners, setEncryptedListeners] = useState<api.Listener[]>([]);
    const [target, setTarget] = useState<string>("");
    const [running, setRunning] = useState(false);
    const [progress, setProgress] = useState<Progresses>(initialProgress);
    const [messageApi, contextHolder] = message.useMessage();

    useEffect(() => {
        if (!open) return;
        ListListeners()
            .then((all) => {
                const enc = all.filter((l) => l.encrypted);
                setEncryptedListeners(enc);
                if (enc.length > 0) setTarget(enc[0].hash);
            })
            .catch((err) => messageApi.error(`load listeners: ${String(err)}`));
        // reset state on open
        setProgress(initialProgress);
        setRunning(false);
    }, [open, messageApi]);

    useEffect(() => {
        if (!running) return;

        const onCompile = (payload: any) => {
            setProgress((p) => ({ ...p, compile: payload?.Progress ?? p.compile }));
        };
        const onCompress = (payload: any) => {
            setProgress((p) => ({ ...p, compress: payload?.Progress ?? p.compress }));
        };
        const onUpload = (payload: any) => {
            const total = payload?.BytesTotal || 1;
            const sent = payload?.BytesSent || 0;
            setProgress((p) => ({
                ...p,
                upload: Math.min(100, Math.round((sent / total) * 100)),
            }));
        };

        EventsOn("notify:upgrade:compile", onCompile);
        EventsOn("notify:upgrade:compress", onCompress);
        EventsOn("notify:upgrade:upload", onUpload);
        return () => {
            EventsOff("notify:upgrade:compile");
            EventsOff("notify:upgrade:compress");
            EventsOff("notify:upgrade:upload");
        };
    }, [running]);

    async function start() {
        if (!target) {
            messageApi.warning("Pick a target listener");
            return;
        }
        setRunning(true);
        setProgress(initialProgress);
        try {
            await UpgradeToTermite(sessionHash, target);
            messageApi.info("Upgrade requested; watch the progress bars.");
        } catch (err) {
            messageApi.error(`upgrade: ${String(err)}`);
            setRunning(false);
        }
    }

    const allDone =
        progress.compile === 100 && progress.compress === 100 && progress.upload === 100;
    const anyError = progress.compile < 0 || progress.compress < 0 || progress.upload < 0;

    return (
        <Modal
            title="Upgrade to Termite (encrypted)"
            open={open}
            onCancel={onClose}
            footer={
                running ? (
                    <Button onClick={onClose}>Close</Button>
                ) : (
                    <Space>
                        <Button onClick={onClose}>Cancel</Button>
                        <Button type="primary" onClick={start} disabled={!target}>
                            Start Upgrade
                        </Button>
                    </Space>
                )
            }
            destroyOnHidden
        >
            {contextHolder}
            {!running ? (
                <>
                    <Text type="secondary">
                        Pick an encrypted listener to act as the callback target. The server will
                        compile a Termite agent, push it through the existing reverse shell, and
                        execute it. The new encrypted session will appear in the Sessions tab.
                    </Text>
                    <div style={{ marginTop: 16 }}>
                        <Select
                            value={target}
                            onChange={setTarget}
                            options={encryptedListeners.map((l) => ({
                                label: `${l.host}:${l.port} (${l.hash.slice(0, 8)}…)`,
                                value: l.hash,
                            }))}
                            style={{ width: "100%" }}
                            placeholder={
                                encryptedListeners.length === 0
                                    ? "No encrypted listeners — create one in the Listeners tab"
                                    : "Select target"
                            }
                            disabled={encryptedListeners.length === 0}
                        />
                    </div>
                </>
            ) : (
                <Space direction="vertical" style={{ width: "100%" }} size="middle">
                    <ProgressRow label="Compile" value={progress.compile} />
                    <ProgressRow label="Compress" value={progress.compress} />
                    <ProgressRow label="Upload" value={progress.upload} />
                    {allDone && (
                        <Alert
                            type="success"
                            showIcon
                            message="Upgrade complete — new termite session should appear shortly."
                        />
                    )}
                    {anyError && (
                        <Alert
                            type="error"
                            showIcon
                            message="Upgrade failed. Check server logs."
                        />
                    )}
                </Space>
            )}
        </Modal>
    );
}

function ProgressRow({ label, value }: { label: string; value: number }) {
    let status: "active" | "success" | "exception" | "normal" = "active";
    if (value === 100) status = "success";
    else if (value < 0) status = "exception";

    return (
        <div>
            <Text>{label}</Text>
            <Progress percent={value < 0 ? 100 : value} status={status} />
        </div>
    );
}
