import { useState } from "react";
import { LogOut, MoreHorizontal, Settings, SlidersHorizontal, User } from "lucide-react";
import { useNavigate, NavLink } from "react-router-dom";

import { SessionUser, logout } from "../lib/auth";
import { palette, space } from "./theme";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

interface Props {
    user: SessionUser;
    serverURL: string;
}

// UserMenu sits at the bottom of the sidebar: avatar + username + role,
// with a more-menu (...) opening a popover with admin / account /
// preferences / logout actions.
//
// Account vs Preferences:
//   · Account → /account → user-self, server-side state (password,
//     identity). Bookmarkable, deep-linkable, distinct from project
//     pages.
//   · Preferences → /preferences → browser-local state (UI density,
//     terminal font, default Fleet view). Lives in localStorage and
//     doesn't sync across devices.
// Surfacing both as separate links makes the scope difference obvious
// instead of conflating them under a single "Settings" entry.
export default function UserMenu({ user, serverURL }: Props) {
    const initials = (user.username || "?").slice(0, 2).toUpperCase();
    const [menuOpen, setMenuOpen] = useState(false);
    const navigate = useNavigate();

    async function handleLogout() {
        setMenuOpen(false);
        await logout();
        navigate("/login", { replace: true });
    }

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
            <div
                className="grid place-items-center flex-shrink-0 size-8 rounded-full border border-border-strong bg-surface-hover text-xs font-semibold text-text-primary"
            >
                {initials}
            </div>
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
                <div style={{ color: palette.textMuted, fontSize: 11 }}>
                    {roleLabel(user.role)}
                </div>
            </div>

            <Popover open={menuOpen} onOpenChange={setMenuOpen}>
                <PopoverTrigger asChild>
                    <button
                        type="button"
                        aria-label="User menu"
                        className="cursor-pointer rounded p-1 text-text-muted hover:bg-surface-hover hover:text-text-primary"
                    >
                        <MoreHorizontal className="size-4" />
                    </button>
                </PopoverTrigger>
                <PopoverContent align="end" side="top" className="w-[220px] p-1">
                    <div className="mb-2 pb-2 border-b border-border px-2 pt-1">
                        <div className="font-semibold text-text-primary text-sm">
                            {user.username}
                        </div>
                        <div className="text-xs text-text-muted">
                            {roleLabel(user.role)} · {hostOf(serverURL)}
                        </div>
                    </div>
                    {user.role === "admin" && (
                        <>
                            <button
                                type="button"
                                className="pl-popover-btn"
                                onClick={() => {
                                    setMenuOpen(false);
                                    navigate("/admin/users");
                                }}
                            >
                                <Settings className="size-3.5" />
                                <span>Manage users</span>
                            </button>
                            <button
                                type="button"
                                className="pl-popover-btn"
                                onClick={() => {
                                    setMenuOpen(false);
                                    navigate("/admin/settings");
                                }}
                            >
                                <Settings className="size-3.5" />
                                <span>Server settings</span>
                            </button>
                        </>
                    )}
                    <NavLink
                        to="/account"
                        className="pl-popover-btn"
                        onClick={() => setMenuOpen(false)}
                    >
                        <User className="size-3.5" />
                        <span>Account</span>
                    </NavLink>
                    <NavLink
                        to="/preferences"
                        className="pl-popover-btn"
                        onClick={() => setMenuOpen(false)}
                    >
                        <SlidersHorizontal className="size-3.5" />
                        <span>Preferences</span>
                    </NavLink>
                    <button type="button" className="pl-popover-btn" onClick={handleLogout}>
                        <LogOut className="size-3.5" />
                        <span>Log out</span>
                    </button>
                </PopoverContent>
            </Popover>
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
