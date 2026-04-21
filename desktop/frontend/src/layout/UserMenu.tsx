import { useState } from "react";
import { Avatar, Button, Form, Input, Modal, Popover, message } from "antd";
import { KeyOutlined, LogoutOutlined, MoreOutlined, SettingOutlined } from "@ant-design/icons";
import { useNavigate } from "react-router-dom";

import { SessionUser, changePassword, logout } from "../lib/auth";
import { palette, space } from "./theme";

interface Props {
    user: SessionUser;
    serverURL: string;
}

// UserMenu sits at the bottom of the sidebar: avatar + username + role,
// with a more-menu (·••) opening a popover with admin/password/logout
// actions. Replaces the old vertical ProfileRail (since nav is now flat
// in the same sidebar, the rail is gone).
export default function UserMenu({ user, serverURL }: Props) {
    const initials = (user.username || "?").slice(0, 2).toUpperCase();
    const [pwOpen, setPwOpen] = useState(false);
    const [pwForm] = Form.useForm<{
        old_password: string;
        new_password: string;
        confirm: string;
    }>();
    const [pwBusy, setPwBusy] = useState(false);
    const [messageApi, contextHolder] = message.useMessage();
    const navigate = useNavigate();

    async function handleLogout() {
        await logout();
        navigate("/login", { replace: true });
    }

    async function handlePasswordChange() {
        const v = await pwForm.validateFields();
        setPwBusy(true);
        try {
            await changePassword(v.old_password, v.new_password);
            messageApi.success("Password updated — please log in again");
            setPwOpen(false);
            pwForm.resetFields();
            navigate("/login", { replace: true });
        } catch (e) {
            messageApi.error(`change password: ${String(e)}`);
        } finally {
            setPwBusy(false);
        }
    }

    const popoverContent = (
        <div style={{ minWidth: 220 }}>
            <div
                style={{
                    marginBottom: space[3],
                    paddingBottom: space[3],
                    borderBottom: `1px solid ${palette.border}`,
                }}
            >
                <div
                    style={{
                        fontWeight: 600,
                        color: palette.textPrimary,
                        fontSize: 13,
                    }}
                >
                    {user.username}
                </div>
                <div style={{ color: palette.textMuted, fontSize: 12 }}>
                    {roleLabel(user.role)} · {hostOf(serverURL)}
                </div>
            </div>
            {user.role === "admin" && (
                <button
                    type="button"
                    className="pl-popover-btn"
                    onClick={() => navigate("/admin/users")}
                >
                    <SettingOutlined />
                    <span>Manage users</span>
                </button>
            )}
            <button
                type="button"
                className="pl-popover-btn"
                onClick={() => setPwOpen(true)}
            >
                <KeyOutlined />
                <span>Change password</span>
            </button>
            <button type="button" className="pl-popover-btn" onClick={handleLogout}>
                <LogoutOutlined />
                <span>Log out</span>
            </button>
        </div>
    );

    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                padding: `${space[2]}px ${space[3]}px`,
                borderTop: `1px solid ${palette.border}`,
            }}
        >
            {contextHolder}
            <Avatar
                size={32}
                style={{
                    backgroundColor: palette.surfaceHover,
                    color: palette.textPrimary,
                    border: `1px solid ${palette.borderStrong}`,
                    fontWeight: 600,
                    fontSize: 12,
                    flexShrink: 0,
                }}
            >
                {initials}
            </Avatar>
            <div style={{ flex: 1, minWidth: 0 }}>
                <div
                    style={{
                        color: palette.textPrimary,
                        fontSize: 13,
                        fontWeight: 500,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                    }}
                >
                    {user.username}
                </div>
                <div
                    style={{
                        color: palette.textMuted,
                        fontSize: 11,
                    }}
                >
                    {roleLabel(user.role)}
                </div>
            </div>
            <Popover content={popoverContent} placement="topRight" trigger="click">
                <button
                    type="button"
                    aria-label="User menu"
                    style={{
                        background: "transparent",
                        border: "none",
                        color: palette.textMuted,
                        cursor: "pointer",
                        padding: 4,
                        borderRadius: 4,
                    }}
                >
                    <MoreOutlined style={{ fontSize: 16 }} />
                </button>
            </Popover>

            <Modal
                title="Change password"
                open={pwOpen}
                onCancel={() => {
                    setPwOpen(false);
                    pwForm.resetFields();
                }}
                footer={[
                    <Button
                        key="cancel"
                        onClick={() => {
                            setPwOpen(false);
                            pwForm.resetFields();
                        }}
                    >
                        Cancel
                    </Button>,
                    <Button
                        key="submit"
                        type="primary"
                        loading={pwBusy}
                        onClick={handlePasswordChange}
                    >
                        Update password
                    </Button>,
                ]}
                destroyOnHidden
            >
                <Form form={pwForm} layout="vertical">
                    <Form.Item
                        name="old_password"
                        label="Current password"
                        rules={[{ required: true }]}
                    >
                        <Input.Password autoFocus />
                    </Form.Item>
                    <Form.Item
                        name="new_password"
                        label="New password"
                        rules={[{ required: true, min: 8, message: "Min 8 chars" }]}
                        extra="Changing your password will sign you out of all other sessions."
                    >
                        <Input.Password />
                    </Form.Item>
                    <Form.Item
                        name="confirm"
                        label="Confirm new password"
                        dependencies={["new_password"]}
                        rules={[
                            { required: true },
                            ({ getFieldValue }) => ({
                                validator(_, v) {
                                    if (!v || v === getFieldValue("new_password")) {
                                        return Promise.resolve();
                                    }
                                    return Promise.reject(new Error("passwords do not match"));
                                },
                            }),
                        ]}
                    >
                        <Input.Password />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}

function roleLabel(role: SessionUser["role"]): string {
    switch (role) {
        case "admin":
            return "Admin";
        case "operator":
            return "Operator";
        case "viewer":
            return "Viewer";
        default:
            return role;
    }
}

function hostOf(url: string): string {
    try {
        return new URL(url).host;
    } catch {
        return url;
    }
}
