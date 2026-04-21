import { useState } from "react";
import { Avatar, Button, Form, Input, Modal, Popover, Tooltip, message } from "antd";
import { KeyOutlined, LogoutOutlined, SettingOutlined, UserOutlined } from "@ant-design/icons";

import { SessionUser, changePassword, logout } from "../lib/auth";
import { layout, palette, space } from "./theme";

// ProfileRail is the leftmost 56px strip. Hosts the currently-logged-in
// user's avatar pinned at the bottom with a popover menu (manage users,
// change password, log out).
//
// The component receives `user` directly rather than reading from the
// session module so tests can render it in isolation.

interface Props {
    user: SessionUser;
    serverURL: string;
    onLoggedOut: () => void;
    // onOpenAdmin is supplied only when the logged-in user has global
    // admin role; presence drives whether the settings icon renders.
    onOpenAdmin?: () => void;
}

export default function ProfileRail({ user, serverURL, onLoggedOut, onOpenAdmin }: Props) {
    const initials = (user.username || "?").slice(0, 2).toUpperCase();
    const [pwOpen, setPwOpen] = useState(false);
    const [pwForm] = Form.useForm<{
        old_password: string;
        new_password: string;
        confirm: string;
    }>();
    const [pwBusy, setPwBusy] = useState(false);
    const [messageApi, contextHolder] = message.useMessage();

    async function handleLogout() {
        await logout();
        onLoggedOut();
    }

    async function handlePasswordChange() {
        const v = await pwForm.validateFields();
        setPwBusy(true);
        try {
            await changePassword(v.old_password, v.new_password);
            messageApi.success("Password updated — please log in again");
            setPwOpen(false);
            pwForm.resetFields();
            onLoggedOut();
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
                <div style={{ fontWeight: 600, color: palette.textPrimary, fontSize: 13 }}>
                    {user.username}
                </div>
                <div style={{ color: palette.textMuted, fontSize: 12 }}>
                    {roleLabel(user.role)} · {new URL(serverURL).host}
                </div>
            </div>
            {onOpenAdmin && (
                <PopoverButton onClick={onOpenAdmin} icon={<SettingOutlined />}>
                    Manage users
                </PopoverButton>
            )}
            <PopoverButton onClick={() => setPwOpen(true)} icon={<KeyOutlined />}>
                Change password
            </PopoverButton>
            <PopoverButton onClick={handleLogout} icon={<LogoutOutlined />}>
                Log out
            </PopoverButton>
        </div>
    );

    return (
        <div
            style={{
                width: layout.profileRailWidth,
                height: "100%",
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                padding: `${space[4]}px 0`,
                gap: space[3],
            }}
        >
            {contextHolder}
            <div style={{ flex: 1 }} />
            <Popover content={popoverContent} placement="rightBottom" trigger="click">
                <Tooltip title={user.username} placement="right">
                    <Avatar
                        style={{
                            backgroundColor: palette.surfaceHover,
                            color: palette.textPrimary,
                            cursor: "pointer",
                            fontWeight: 600,
                            border: `1px solid ${palette.border}`,
                        }}
                        icon={initials ? undefined : <UserOutlined />}
                    >
                        {initials}
                    </Avatar>
                </Tooltip>
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

function PopoverButton({
    children,
    icon,
    onClick,
}: {
    children: React.ReactNode;
    icon: React.ReactNode;
    onClick: () => void;
}) {
    return (
        <button type="button" onClick={onClick} className="pl-popover-btn">
            {icon}
            <span>{children}</span>
        </button>
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
