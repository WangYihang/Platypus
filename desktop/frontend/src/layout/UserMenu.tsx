import { useState } from "react";
import { Key, Loader2, LogOut, MoreHorizontal, Settings } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNavigate } from "react-router-dom";

import { SessionUser, changePassword, logout } from "../lib/auth";
import { palette, space } from "./theme";

import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import {
    Form,
    FormControl,
    FormDescription,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

// Zod refinement to check confirm matches new_password. Runs after the
// individual-field validation, so we see match errors only once both
// fields look otherwise valid.
const passwordSchema = z
    .object({
        old_password: z.string().min(1, "current password is required"),
        new_password: z.string().min(8, "Min 8 chars"),
        confirm: z.string().min(1, "required"),
    })
    .refine((v) => v.confirm === v.new_password, {
        path: ["confirm"],
        message: "passwords do not match",
    });
type PasswordFormValues = z.infer<typeof passwordSchema>;

interface Props {
    user: SessionUser;
    serverURL: string;
}

// UserMenu sits at the bottom of the sidebar: avatar + username + role,
// with a more-menu (...) opening a popover with admin/password/logout
// actions.
export default function UserMenu({ user, serverURL }: Props) {
    const initials = (user.username || "?").slice(0, 2).toUpperCase();
    const [pwOpen, setPwOpen] = useState(false);
    const [menuOpen, setMenuOpen] = useState(false);
    const navigate = useNavigate();

    const pwForm = useForm<PasswordFormValues>({
        resolver: zodResolver(passwordSchema),
        defaultValues: { old_password: "", new_password: "", confirm: "" },
    });

    async function handleLogout() {
        setMenuOpen(false);
        await logout();
        navigate("/login", { replace: true });
    }

    async function handlePasswordChange(v: PasswordFormValues) {
        try {
            await changePassword(v.old_password, v.new_password);
            toast.success("Password updated — please log in again");
            setPwOpen(false);
            pwForm.reset({ old_password: "", new_password: "", confirm: "" });
            navigate("/login", { replace: true });
        } catch (e) {
            toast.error(`change password: ${String(e)}`);
        }
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
                    )}
                    <button
                        type="button"
                        className="pl-popover-btn"
                        onClick={() => {
                            setMenuOpen(false);
                            setPwOpen(true);
                        }}
                    >
                        <Key className="size-3.5" />
                        <span>Change password</span>
                    </button>
                    <button type="button" className="pl-popover-btn" onClick={handleLogout}>
                        <LogOut className="size-3.5" />
                        <span>Log out</span>
                    </button>
                </PopoverContent>
            </Popover>

            <Dialog
                open={pwOpen}
                onOpenChange={(o) => {
                    setPwOpen(o);
                    if (!o) pwForm.reset({ old_password: "", new_password: "", confirm: "" });
                }}
            >
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>Change password</DialogTitle>
                        <DialogDescription>
                            Changing your password will sign you out of all other sessions.
                        </DialogDescription>
                    </DialogHeader>
                    <Form {...pwForm}>
                        <form
                            onSubmit={pwForm.handleSubmit(handlePasswordChange)}
                            className="space-y-4"
                        >
                            <FormField
                                control={pwForm.control}
                                name="old_password"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Current password</FormLabel>
                                        <FormControl>
                                            <Input type="password" autoFocus {...field} />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={pwForm.control}
                                name="new_password"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>New password</FormLabel>
                                        <FormControl>
                                            <Input type="password" {...field} />
                                        </FormControl>
                                        <FormDescription>Min 8 characters.</FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={pwForm.control}
                                name="confirm"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Confirm new password</FormLabel>
                                        <FormControl>
                                            <Input type="password" {...field} />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <DialogFooter>
                                <Button
                                    type="button"
                                    variant="outline"
                                    onClick={() => setPwOpen(false)}
                                >
                                    Cancel
                                </Button>
                                <Button
                                    type="submit"
                                    disabled={pwForm.formState.isSubmitting}
                                >
                                    {pwForm.formState.isSubmitting && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Update password
                                </Button>
                            </DialogFooter>
                        </form>
                    </Form>
                </DialogContent>
            </Dialog>
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
