import { useCallback, useEffect, useState } from "react";
import {
    Alert,
    Button,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Table,
    message,
} from "antd";
import { DeleteOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import StatusPill from "../../components/StatusPill";
import PageHeader from "../../components/PageHeader";
import { space } from "../../layout/theme";
import { UserRow, createUser, deleteUser, listUsers, updateUser } from "../../lib/api";

const ROLE_TONE: Record<UserRow["role"], "danger" | "info" | "neutral"> = {
    admin: "danger",
    operator: "info",
    viewer: "neutral",
};

// AdminUsers is the /users admin surface: list / create / change-role /
// change-password / delete. Single-table view so admins can scan the
// whole roster without scrolling through cards.
export default function AdminUsers() {
    const [users, setUsers] = useState<UserRow[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [createOpen, setCreateOpen] = useState(false);
    const [pwOpen, setPwOpen] = useState<string | null>(null);
    const [createForm] = Form.useForm<{
        username: string;
        password: string;
        role: UserRow["role"];
    }>();
    const [pwForm] = Form.useForm<{ password: string }>();
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setUsers(await listUsers());
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function handleCreate() {
        const v = await createForm.validateFields();
        try {
            await createUser(v.username, v.password, v.role);
            messageApi.success(`Created ${v.username}`);
            setCreateOpen(false);
            createForm.resetFields();
            refresh();
        } catch (e) {
            messageApi.error(`create: ${String(e)}`);
        }
    }

    function handleDelete(u: UserRow) {
        Modal.confirm({
            title: `Delete user ${u.username}?`,
            content: "Their refresh tokens are revoked and they can no longer log in.",
            okText: "Delete",
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await deleteUser(u.id);
                    messageApi.success(`Deleted ${u.username}`);
                    refresh();
                } catch (e) {
                    messageApi.error(`delete: ${String(e)}`);
                }
            },
        });
    }

    async function handleRoleChange(u: UserRow, role: UserRow["role"]) {
        try {
            await updateUser(u.id, { role });
            messageApi.success(`Updated ${u.username} role`);
            refresh();
        } catch (e) {
            messageApi.error(`role: ${String(e)}`);
        }
    }

    async function handlePasswordReset() {
        const v = await pwForm.validateFields();
        if (!pwOpen) return;
        try {
            await updateUser(pwOpen, { password: v.password });
            messageApi.success("Password updated; existing sessions revoked");
            setPwOpen(null);
            pwForm.resetFields();
        } catch (e) {
            messageApi.error(`reset: ${String(e)}`);
        }
    }

    const columns: ColumnsType<UserRow> = [
        { title: "Username", dataIndex: "username" },
        {
            title: "Role",
            dataIndex: "role",
            render: (role: UserRow["role"], u) => (
                <Select
                    size="small"
                    value={role}
                    style={{ minWidth: 130 }}
                    onChange={(v) => handleRoleChange(u, v)}
                    options={(["admin", "operator", "viewer"] as UserRow["role"][]).map((r) => ({
                        label: <StatusPill tone={ROLE_TONE[r]}>{r}</StatusPill>,
                        value: r,
                    }))}
                />
            ),
            width: 180,
        },
        {
            title: "",
            render: (_, u) => (
                <Space>
                    <Button size="small" type="link" onClick={() => setPwOpen(u.id)}>
                        Reset password
                    </Button>
                    <Button
                        size="small"
                        type="link"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() => handleDelete(u)}
                    >
                        Delete
                    </Button>
                </Space>
            ),
            width: 260,
        },
    ];

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title="Users"
                subtitle="Manage who can log in and what they can do"
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
                        <Button
                            size="small"
                            type="primary"
                            icon={<PlusOutlined />}
                            onClick={() => setCreateOpen(true)}
                        >
                            New user
                        </Button>
                    </Space>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                <div>
                    {error && (
                        <Alert type="error" message={error} style={{ marginBottom: space[4] }} />
                    )}
                    {users && users.length === 0 ? (
                        <EmptyState
                            title="No users"
                            description="Create the first user via New user."
                            action={
                                <Button
                                    type="primary"
                                    icon={<PlusOutlined />}
                                    onClick={() => setCreateOpen(true)}
                                >
                                    New user
                                </Button>
                            }
                        />
                    ) : (
                        <Card padding={0}>
                            <Table
                                rowKey="id"
                                columns={columns}
                                dataSource={users ?? []}
                                loading={!users}
                                pagination={false}
                                size="small"
                                bordered={false}
                            />
                        </Card>
                    )}
                </div>
            </div>

            <Modal
                title="New user"
                open={createOpen}
                onOk={handleCreate}
                onCancel={() => setCreateOpen(false)}
                okText="Create"
                destroyOnHidden
            >
                <Form
                    form={createForm}
                    layout="vertical"
                    initialValues={{ role: "operator" }}
                >
                    <Form.Item name="username" label="Username" rules={[{ required: true }]}>
                        <Input autoFocus />
                    </Form.Item>
                    <Form.Item
                        name="password"
                        label="Initial password"
                        rules={[{ required: true, min: 8, message: "Min 8 chars" }]}
                        extra="The user can change this after logging in."
                    >
                        <Input.Password />
                    </Form.Item>
                    <Form.Item name="role" label="Role" rules={[{ required: true }]}>
                        <Select
                            options={(["admin", "operator", "viewer"] as UserRow["role"][]).map(
                                (r) => ({
                                    label: <StatusPill tone={ROLE_TONE[r]}>{r}</StatusPill>,
                                    value: r,
                                }),
                            )}
                        />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title="Reset password"
                open={pwOpen !== null}
                onOk={handlePasswordReset}
                onCancel={() => setPwOpen(null)}
                okText="Reset"
                destroyOnHidden
            >
                <Form form={pwForm} layout="vertical">
                    <Form.Item
                        name="password"
                        label="New password"
                        rules={[{ required: true, min: 8, message: "Min 8 chars" }]}
                        extra="All of the user's active sessions are invalidated."
                    >
                        <Input.Password autoFocus />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
