import { useCallback, useEffect, useMemo, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    Modal,
    Segmented,
    Spin,
    Table,
    Tabs,
    Tag,
    Typography,
    message,
} from "antd";
import {
    CopyOutlined,
    DeleteOutlined,
    PlusOutlined,
    ReloadOutlined,
    ThunderboltOutlined,
} from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    InstallArtifactListItem,
    IssueInstallResponse,
    IssuePATResponse,
    PATTokenListItem,
    issueInstallArtifact,
    issuePAT,
    listInstallArtifacts,
    listPATTokens,
    revokeInstallArtifact,
    revokePAT,
} from "../lib/api";
import { fromNow } from "../lib/time";

// EnrollmentPage bundles three related admin surfaces onto one page
// because they share the same mental model — "hand an agent something
// short-lived so it can join the mesh":
//
//  1. Install commands: one-shot `curl ... | sh` bootstraps that
//     atomically mint a PAT the first time they're fetched. Easiest
//     path for 99% of deployments.
//  2. Raw PAT tokens: for scripted / CI flows that can't use the
//     install-command shape.
//  3. (future tab) Audit export: compliance ndjson/csv dump of every
//     redemption and admin action. Stub in the "Export" button in the
//     page header for now; full UI can follow.
//
// Every table row shows derived status, not a mutable column, so
// refreshing the view always reflects what's true.
export default function EnrollmentPage() {
    const project = useCurrentProject();
    const [tab, setTab] = useState<"install" | "tokens">("install");
    const [messageApi, contextHolder] = message.useMessage();

    return (
        <>
            {contextHolder}
            <PageHeader
                title="Enrollment"
                subtitle="Distribute agents with one-shot install commands or raw PATs"
            />
            <Tabs
                activeKey={tab}
                onChange={(k) => setTab(k as "install" | "tokens")}
                items={[
                    {
                        key: "install",
                        label: (
                            <span>
                                <ThunderboltOutlined /> Install commands
                            </span>
                        ),
                        children: <InstallPanel projectID={project.id} messageApi={messageApi} />,
                    },
                    {
                        key: "tokens",
                        label: "PAT tokens",
                        children: <PATPanel projectID={project.id} messageApi={messageApi} />,
                    },
                ]}
            />
        </>
    );
}

// --- Install commands tab ---------------------------------------------

interface PanelProps {
    projectID: string;
    messageApi: ReturnType<typeof message.useMessage>[0];
}

function InstallPanel({ projectID, messageApi }: PanelProps) {
    const [rows, setRows] = useState<InstallArtifactListItem[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [filter, setFilter] = useState<"active" | "all">("active");
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssueInstallResponse | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listInstallArtifacts(projectID, filter === "all");
            setRows(data);
            setError(null);
        } catch (e) {
            setError(String(e));
            messageApi.error(`list install artifacts: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [projectID, filter, messageApi]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const columns: ColumnsType<InstallArtifactListItem> = [
        {
            title: "Download ID",
            dataIndex: "download_id",
            render: (v: string) => <Mono size={12}>{v}</Mono>,
            width: 260,
        },
        {
            title: "Target",
            render: (_: unknown, r: InstallArtifactListItem) =>
                r.target_os || r.target_arch
                    ? `${r.target_os || "any"}/${r.target_arch || "any"}`
                    : <span style={{ color: palette.textMuted }}>—</span>,
            width: 120,
        },
        {
            title: "Server",
            dataIndex: "server_endpoint",
            render: (v: string) => <Mono size={12}>{v}</Mono>,
        },
        {
            title: "Status",
            dataIndex: "status",
            render: (s: InstallArtifactListItem["status"]) => <StatusTag status={s} />,
            width: 110,
        },
        {
            title: "Expires",
            dataIndex: "expires_at",
            render: (v: string) => fromNow(v),
            width: 120,
        },
        {
            title: "Consumed",
            render: (_: unknown, r: InstallArtifactListItem) =>
                r.consumed_at ? (
                    <span>
                        {fromNow(r.consumed_at)}
                        {r.consumed_ip ? ` · ${r.consumed_ip}` : ""}
                    </span>
                ) : (
                    <span style={{ color: palette.textMuted }}>—</span>
                ),
        },
        {
            title: "",
            width: 80,
            render: (_: unknown, r: InstallArtifactListItem) =>
                !r.revoked && !r.consumed_at ? (
                    <Button
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() =>
                            Modal.confirm({
                                title: "Revoke install link?",
                                content: "The curl command will stop working immediately.",
                                okText: "Revoke",
                                okButtonProps: { danger: true },
                                onOk: async () => {
                                    try {
                                        await revokeInstallArtifact(projectID, r.download_id);
                                        messageApi.success("Install link revoked");
                                        refresh();
                                    } catch (e) {
                                        messageApi.error(`revoke: ${String(e)}`);
                                    }
                                },
                            })
                        }
                    />
                ) : null,
        },
    ];

    return (
        <>
            <Toolbar
                left={
                    <Segmented
                        value={filter}
                        onChange={(v) => setFilter(v as "active" | "all")}
                        options={[
                            { label: "Active", value: "active" },
                            { label: "All", value: "all" },
                        ]}
                    />
                }
                right={
                    <>
                        <Button icon={<ReloadOutlined />} onClick={refresh}>
                            Refresh
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={() => setIssueOpen(true)}>
                            Generate install command
                        </Button>
                    </>
                }
            />
            {error && <Alert type="error" message={error} style={{ marginBottom: space[3] }} />}
            <Card padding={0}>
                {rows === null ? (
                    <CenterPad>
                        <Spin />
                    </CenterPad>
                ) : rows.length === 0 ? (
                    <EmptyState
                        title="No install commands yet"
                        description="Generate one for a host in this project."
                    />
                ) : (
                    <Table
                        rowKey="download_id"
                        columns={columns}
                        dataSource={rows}
                        pagination={{ pageSize: 25 }}
                        size="small"
                        loading={loading}
                    />
                )}
            </Card>

            <IssueInstallModal
                open={issueOpen}
                onClose={() => {
                    setIssueOpen(false);
                    refresh();
                }}
                onIssued={(r) => {
                    setLastIssued(r);
                    setIssueOpen(false);
                    refresh();
                }}
                projectID={projectID}
                messageApi={messageApi}
            />

            <IssuedInstallModal
                result={lastIssued}
                onClose={() => setLastIssued(null)}
                messageApi={messageApi}
            />
        </>
    );
}

function IssueInstallModal({
    open,
    onClose,
    onIssued,
    projectID,
    messageApi,
}: {
    open: boolean;
    onClose: () => void;
    onIssued: (r: IssueInstallResponse) => void;
    projectID: string;
    messageApi: ReturnType<typeof message.useMessage>[0];
}) {
    const [form] = Form.useForm<{
        server_endpoint: string;
        target_os?: string;
        target_arch?: string;
        ttl_seconds?: number;
    }>();
    const [busy, setBusy] = useState(false);

    async function submit() {
        const v = await form.validateFields();
        setBusy(true);
        try {
            const r = await issueInstallArtifact(projectID, v);
            onIssued(r);
            form.resetFields();
        } catch (e) {
            messageApi.error(`issue: ${String(e)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <Modal
            title="Generate install command"
            open={open}
            onCancel={onClose}
            onOk={submit}
            okText="Generate"
            confirmLoading={busy}
        >
            <Form form={form} layout="vertical">
                <Form.Item
                    label="Agent should dial"
                    name="server_endpoint"
                    rules={[{ required: true, message: "required" }]}
                    tooltip="host:port that the agent will connect back to (usually one of your TCP listeners)"
                >
                    <Input placeholder="203.0.113.5:13337" />
                </Form.Item>
                <Form.Item label="Target OS" name="target_os">
                    <Input placeholder="linux (optional)" />
                </Form.Item>
                <Form.Item label="Target arch" name="target_arch">
                    <Input placeholder="amd64 (optional)" />
                </Form.Item>
                <Form.Item
                    label="Download link TTL (seconds)"
                    name="ttl_seconds"
                    tooltip="Default 300 (5 min)"
                >
                    <Input type="number" placeholder="300" />
                </Form.Item>
            </Form>
        </Modal>
    );
}

function IssuedInstallModal({
    result,
    onClose,
    messageApi,
}: {
    result: IssueInstallResponse | null;
    onClose: () => void;
    messageApi: ReturnType<typeof message.useMessage>[0];
}) {
    if (!result) return null;
    return (
        <Modal
            title="Install command generated"
            open
            onCancel={onClose}
            footer={<Button onClick={onClose}>Done</Button>}
            width={640}
        >
            <Alert
                type="warning"
                message="This is the only time the token is shown"
                description="After closing this dialog the server discards the plaintext; further copies are not possible."
                style={{ marginBottom: space[3] }}
            />
            <Typography.Paragraph style={{ marginBottom: space[2], color: palette.textMuted, fontSize: 12 }}>
                Run on the target machine:
            </Typography.Paragraph>
            <div
                style={{
                    fontFamily: "monospace",
                    fontSize: 12,
                    background: palette.surface,
                    border: `1px solid ${palette.border}`,
                    borderRadius: 6,
                    padding: space[3],
                    wordBreak: "break-all",
                }}
            >
                {result.install_command}
            </div>
            <div style={{ marginTop: space[3], display: "flex", justifyContent: "flex-end" }}>
                <Button
                    icon={<CopyOutlined />}
                    onClick={async () => {
                        await navigator.clipboard.writeText(result.install_command);
                        messageApi.success("Copied to clipboard");
                    }}
                >
                    Copy command
                </Button>
            </div>
        </Modal>
    );
}

// --- PAT tokens tab ---------------------------------------------------

function PATPanel({ projectID, messageApi }: PanelProps) {
    const [rows, setRows] = useState<PATTokenListItem[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [filter, setFilter] = useState<"active" | "all">("active");
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssuePATResponse | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listPATTokens(projectID, filter === "all");
            setRows(data);
            setError(null);
        } catch (e) {
            setError(String(e));
            messageApi.error(`list tokens: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [projectID, filter, messageApi]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const columns: ColumnsType<PATTokenListItem> = [
        {
            title: "Token ID",
            dataIndex: "token_id",
            render: (v: string) => <Mono size={12}>{v}</Mono>,
            width: 260,
        },
        {
            title: "Description",
            dataIndex: "description",
            render: (v?: string) => v || <span style={{ color: palette.textMuted }}>—</span>,
        },
        {
            title: "Status",
            dataIndex: "status",
            render: (s: PATTokenListItem["status"]) => <StatusTag status={s} />,
            width: 110,
        },
        {
            title: "Uses",
            render: (_: unknown, r: PATTokenListItem) => `${r.uses}/${r.max_uses}`,
            width: 80,
        },
        {
            title: "Expires",
            dataIndex: "expires_at",
            render: (v: string) => fromNow(v),
            width: 120,
        },
        {
            title: "",
            width: 80,
            render: (_: unknown, r: PATTokenListItem) =>
                !r.revoked && r.status === "pending" ? (
                    <Button
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() =>
                            Modal.confirm({
                                title: "Revoke PAT?",
                                content: "The token will be rejected on any subsequent enrollment attempt.",
                                okText: "Revoke",
                                okButtonProps: { danger: true },
                                onOk: async () => {
                                    try {
                                        await revokePAT(projectID, r.token_id);
                                        messageApi.success("PAT revoked");
                                        refresh();
                                    } catch (e) {
                                        messageApi.error(`revoke: ${String(e)}`);
                                    }
                                },
                            })
                        }
                    />
                ) : null,
        },
    ];

    return (
        <>
            <Toolbar
                left={
                    <Segmented
                        value={filter}
                        onChange={(v) => setFilter(v as "active" | "all")}
                        options={[
                            { label: "Active", value: "active" },
                            { label: "All", value: "all" },
                        ]}
                    />
                }
                right={
                    <>
                        <Button icon={<ReloadOutlined />} onClick={refresh}>
                            Refresh
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={() => setIssueOpen(true)}>
                            Issue PAT
                        </Button>
                    </>
                }
            />
            {error && <Alert type="error" message={error} style={{ marginBottom: space[3] }} />}
            <Card padding={0}>
                {rows === null ? (
                    <CenterPad>
                        <Spin />
                    </CenterPad>
                ) : rows.length === 0 ? (
                    <EmptyState
                        title="No PATs issued yet"
                        description="Prefer the install command tab unless you need raw tokens for a CI pipeline."
                    />
                ) : (
                    <Table
                        rowKey="token_id"
                        columns={columns}
                        dataSource={rows}
                        pagination={{ pageSize: 25 }}
                        size="small"
                        loading={loading}
                    />
                )}
            </Card>

            <IssuePATModal
                open={issueOpen}
                onClose={() => {
                    setIssueOpen(false);
                    refresh();
                }}
                onIssued={(r) => {
                    setLastIssued(r);
                    setIssueOpen(false);
                    refresh();
                }}
                projectID={projectID}
                messageApi={messageApi}
            />
            <IssuedPATModal
                result={lastIssued}
                onClose={() => setLastIssued(null)}
                messageApi={messageApi}
            />
        </>
    );
}

function IssuePATModal({
    open,
    onClose,
    onIssued,
    projectID,
    messageApi,
}: {
    open: boolean;
    onClose: () => void;
    onIssued: (r: IssuePATResponse) => void;
    projectID: string;
    messageApi: ReturnType<typeof message.useMessage>[0];
}) {
    const [form] = Form.useForm<{
        description?: string;
        ttl_seconds?: number;
        max_uses?: number;
        binding_machine_id?: string;
    }>();
    const [busy, setBusy] = useState(false);

    async function submit() {
        const v = await form.validateFields();
        setBusy(true);
        try {
            const r = await issuePAT(projectID, v);
            onIssued(r);
            form.resetFields();
        } catch (e) {
            messageApi.error(`issue: ${String(e)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <Modal
            title="Issue PAT"
            open={open}
            onCancel={onClose}
            onOk={submit}
            okText="Issue"
            confirmLoading={busy}
        >
            <Form form={form} layout="vertical">
                <Form.Item label="Description" name="description" tooltip="Free-form note shown in the list">
                    <Input placeholder="Deploy for web-01" />
                </Form.Item>
                <Form.Item label="TTL (seconds)" name="ttl_seconds" tooltip="Default 3600 (1h)">
                    <Input type="number" placeholder="3600" />
                </Form.Item>
                <Form.Item label="Max uses" name="max_uses" tooltip="Default 1 (single-use)">
                    <Input type="number" placeholder="1" />
                </Form.Item>
                <Form.Item
                    label="Binding machine ID"
                    name="binding_machine_id"
                    tooltip="If set, the PAT is only accepted from a machine whose /etc/machine-id matches."
                >
                    <Input placeholder="(optional)" />
                </Form.Item>
            </Form>
        </Modal>
    );
}

function IssuedPATModal({
    result,
    onClose,
    messageApi,
}: {
    result: IssuePATResponse | null;
    onClose: () => void;
    messageApi: ReturnType<typeof message.useMessage>[0];
}) {
    if (!result) return null;
    return (
        <Modal
            title="PAT issued"
            open
            onCancel={onClose}
            footer={<Button onClick={onClose}>Done</Button>}
            width={640}
        >
            <Alert
                type="warning"
                message="This is the only time the token is shown"
                description="Copy it now — after closing this dialog the server cannot show it again."
                style={{ marginBottom: space[3] }}
            />
            <div
                style={{
                    fontFamily: "monospace",
                    fontSize: 12,
                    background: palette.surface,
                    border: `1px solid ${palette.border}`,
                    borderRadius: 6,
                    padding: space[3],
                    wordBreak: "break-all",
                }}
            >
                {result.token}
            </div>
            <div style={{ marginTop: space[3], display: "flex", justifyContent: "flex-end" }}>
                <Button
                    icon={<CopyOutlined />}
                    onClick={async () => {
                        await navigator.clipboard.writeText(result.token);
                        messageApi.success("Copied to clipboard");
                    }}
                >
                    Copy token
                </Button>
            </div>
        </Modal>
    );
}

// --- Shared bits -----------------------------------------------------

function StatusTag({ status }: { status: "pending" | "consumed" | "expired" | "revoked" }) {
    const colour = useMemo(() => {
        switch (status) {
            case "pending":
                return "green";
            case "consumed":
                return "blue";
            case "expired":
                return "orange";
            case "revoked":
                return "red";
        }
    }, [status]);
    return <Tag color={colour}>{status}</Tag>;
}

function CenterPad({ children }: { children: React.ReactNode }) {
    return (
        <div style={{ display: "flex", justifyContent: "center", padding: space[6] }}>{children}</div>
    );
}
