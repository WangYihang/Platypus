import { useCallback, useEffect, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Modal,
    Select,
    Space,
    Table,
    message,
} from "antd";
import { DeleteOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import StatusPill from "../components/StatusPill";
import PageHeader from "../components/PageHeader";
import { palette, space } from "../layout/theme";
import {
    Project,
    ProjectMember,
    UserRow,
    addProjectMember,
    listProjectMembers,
    listUsers,
    removeProjectMember,
} from "../lib/api";
import { getSessionUser } from "../lib/auth";

interface Props {
    project: Project;
}

const ROLE_TONE: Record<ProjectMember["role"], "danger" | "info" | "neutral"> = {
    admin: "danger",
    operator: "info",
    viewer: "neutral",
};

// ProjectMembers is the per-project ACL editor. Global admins can add
// anyone; project-admins can change roles and remove members but not
// add new ones — the backend's /users list is admin-only.
export default function ProjectMembers({ project }: Props) {
    const [members, setMembers] = useState<ProjectMember[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [addOpen, setAddOpen] = useState(false);
    const [candidates, setCandidates] = useState<UserRow[] | null>(null);
    const [addForm] = Form.useForm<{ user_id: string; role: ProjectMember["role"] }>();
    const [messageApi, contextHolder] = message.useMessage();

    const me = getSessionUser();
    const canAdd = me?.role === "admin";

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setMembers(await listProjectMembers(project.id));
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [project.id]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function openAddModal() {
        setAddOpen(true);
        if (!candidates) {
            try {
                setCandidates(await listUsers());
            } catch (e) {
                messageApi.error(`load users: ${String(e)}`);
            }
        }
    }

    async function handleAdd() {
        const v = await addForm.validateFields();
        try {
            await addProjectMember(project.id, v.user_id, v.role);
            messageApi.success("Member added");
            setAddOpen(false);
            addForm.resetFields();
            refresh();
        } catch (e) {
            messageApi.error(`add: ${String(e)}`);
        }
    }

    async function handleRoleChange(m: ProjectMember, role: ProjectMember["role"]) {
        try {
            await addProjectMember(project.id, m.user_id, role);
            messageApi.success(`${m.username} → ${role}`);
            refresh();
        } catch (e) {
            messageApi.error(`role: ${String(e)}`);
        }
    }

    function handleRemove(m: ProjectMember) {
        Modal.confirm({
            title: `Remove ${m.username} from ${project.slug}?`,
            content:
                "They lose access to this project. Other projects and their global account are untouched.",
            okText: "Remove",
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await removeProjectMember(project.id, m.user_id);
                    messageApi.success(`${m.username} removed`);
                    refresh();
                } catch (e) {
                    messageApi.error(`remove: ${String(e)}`);
                }
            },
        });
    }

    const existingIds = new Set(members?.map((m) => m.user_id));
    const availableCandidates = (candidates ?? []).filter((u) => !existingIds.has(u.id));

    const columns: ColumnsType<ProjectMember> = [
        { title: "User", dataIndex: "username" },
        {
            title: "Role",
            dataIndex: "role",
            render: (role: ProjectMember["role"], m) => (
                <Select
                    size="small"
                    value={role}
                    style={{ minWidth: 130 }}
                    onChange={(v) => handleRoleChange(m, v)}
                    options={[
                        { label: <StatusPill tone="danger">admin</StatusPill>, value: "admin" },
                        { label: <StatusPill tone="info">operator</StatusPill>, value: "operator" },
                        { label: <StatusPill tone="neutral">viewer</StatusPill>, value: "viewer" },
                    ]}
                />
            ),
            width: 180,
        },
        {
            title: "",
            render: (_, m) => (
                <Button
                    size="small"
                    type="link"
                    danger
                    icon={<DeleteOutlined />}
                    onClick={() => handleRemove(m)}
                >
                    Remove
                </Button>
            ),
            width: 140,
        },
    ];

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title={`${project.name} · members`}
                subtitle={`${members?.length ?? 0} member(s)`}
                actions={
                    <Space size={space[2]}>
                        <Button
                            size="small"
                            icon={<ReloadOutlined />}
                            loading={loading}
                            onClick={refresh}
                        >
                            Refresh
                        </Button>
                        {canAdd && (
                            <Button
                                size="small"
                                type="primary"
                                icon={<PlusOutlined />}
                                onClick={openAddModal}
                            >
                                Add member
                            </Button>
                        )}
                    </Space>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[6] }}>
                <div style={{ maxWidth: 1200, margin: "0 auto" }}>
                    {error && <Alert type="error" message={error} style={{ marginBottom: space[4] }} />}
                    {!canAdd && (
                        <p
                            style={{
                                margin: `0 0 ${space[4]}px`,
                                color: palette.textSecondary,
                                fontSize: 13,
                                lineHeight: 1.5,
                            }}
                        >
                            Adding new members requires global admin. Ask one to add the user,
                            then you can adjust their project role from this table.
                        </p>
                    )}
                    {members && members.length === 0 ? (
                        <EmptyState
                            title="No members"
                            description={
                                canAdd
                                    ? "Add a user to grant them access to this project."
                                    : "Ask a global admin to add a user to this project."
                            }
                        />
                    ) : (
                        <Card padding={0}>
                            <Table
                                rowKey="user_id"
                                columns={columns}
                                dataSource={members ?? []}
                                loading={!members}
                                pagination={false}
                                size="small"
                                bordered={false}
                            />
                        </Card>
                    )}
                </div>
            </div>

            <Modal
                title="Add project member"
                open={addOpen}
                onOk={handleAdd}
                onCancel={() => {
                    setAddOpen(false);
                    addForm.resetFields();
                }}
                okText="Add"
                destroyOnHidden
            >
                <Form
                    form={addForm}
                    layout="vertical"
                    initialValues={{ role: "operator" }}
                >
                    <Form.Item
                        name="user_id"
                        label="User"
                        rules={[{ required: true, message: "pick a user" }]}
                    >
                        <Select
                            showSearch
                            placeholder="Select a user"
                            options={availableCandidates.map((u) => ({
                                label: (
                                    <span style={{ color: palette.textPrimary }}>
                                        {u.username}{" "}
                                        <span
                                            style={{
                                                color: palette.textSecondary,
                                                fontSize: 11,
                                            }}
                                        >
                                            ({u.role})
                                        </span>
                                    </span>
                                ),
                                value: u.id,
                                label_text: u.username,
                            }))}
                            filterOption={(input, opt) => {
                                const lt = (opt as unknown as { label_text?: string }).label_text;
                                return (lt ?? "").toLowerCase().includes(input.toLowerCase());
                            }}
                        />
                    </Form.Item>
                    <Form.Item name="role" label="Project role" rules={[{ required: true }]}>
                        <Select
                            options={(["admin", "operator", "viewer"] as ProjectMember["role"][]).map(
                                (r) => ({
                                    label: <StatusPill tone={ROLE_TONE[r]}>{r}</StatusPill>,
                                    value: r,
                                }),
                            )}
                        />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
