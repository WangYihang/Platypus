import { Avatar, Popover, Tooltip } from "antd";
import { LogoutOutlined, SettingOutlined, UserOutlined } from "@ant-design/icons";

import { SessionUser, logout } from "../lib/auth";
import { layout, palette } from "./theme";

// ProfileRail is the leftmost 56px strip. In the final design it will
// host per-server profile avatars (Slack's workspace switcher analogue);
// for now the only inhabitant is the currently-logged-in user's avatar,
// pinned at the bottom with a popover menu for log out.
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

    async function handleLogout() {
        await logout();
        onLoggedOut();
    }

    const popoverContent = (
        <div style={{ minWidth: 220 }}>
            <div style={{ marginBottom: 12 }}>
                <div style={{ fontWeight: 600 }}>{user.username}</div>
                <div style={{ color: palette.textSecondary, fontSize: 12 }}>
                    {roleLabel(user.role)} · {new URL(serverURL).host}
                </div>
            </div>
            {onOpenAdmin && (
                <button type="button" onClick={onOpenAdmin} style={popoverButtonStyle}>
                    <SettingOutlined />
                    <span>Manage users</span>
                </button>
            )}
            <button
                type="button"
                onClick={handleLogout}
                style={{ ...popoverButtonStyle, marginTop: 8 }}
            >
                <LogoutOutlined />
                <span>Log out</span>
            </button>
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
                padding: "16px 0",
                gap: 12,
            }}
        >
            <div style={{ flex: 1 }} />
            <Popover content={popoverContent} placement="rightBottom" trigger="click">
                <Tooltip title={user.username} placement="right">
                    <Avatar
                        style={{
                            backgroundColor: palette.accent,
                            cursor: "pointer",
                            fontWeight: 600,
                        }}
                        icon={initials ? undefined : <UserOutlined />}
                    >
                        {initials}
                    </Avatar>
                </Tooltip>
            </Popover>
        </div>
    );
}

const popoverButtonStyle: React.CSSProperties = {
    display: "flex",
    alignItems: "center",
    gap: 8,
    width: "100%",
    padding: "8px 12px",
    border: `1px solid ${palette.border}`,
    background: "transparent",
    color: palette.textPrimary,
    cursor: "pointer",
    borderRadius: 4,
};

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
