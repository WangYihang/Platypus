import { Loader2 } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../components/Card";
import DataList from "../components/DataList";
import Mono from "../components/Mono";
import PageShell from "../components/PageShell";
import { palette, space } from "../layout/theme";
import { changePassword, getSessionUser } from "../lib/auth";
import { humanizeError } from "../lib/humanizeError";

import { Button } from "@/components/ui/button";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import AccountTokensTab from "./account/AccountTokensTab";

// Account is the home of *user-level, server-side* settings:
//
//   • Identity   — read-only username / role / id card
//   • Password   — change-password form (session reset on submit)
//   • Tokens     — manage user-self PATs (`pat_*`)
//
// Distinct from /preferences (browser-local).
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

export default function Account() {
    const user = getSessionUser();
    const navigate = useNavigate();

    const pwForm = useForm<PasswordFormValues>({
        resolver: zodResolver(passwordSchema),
        defaultValues: { old_password: "", new_password: "", confirm: "" },
    });

    async function handlePasswordChange(v: PasswordFormValues) {
        try {
            await changePassword(v.old_password, v.new_password);
            toast.success("Password updated — please log in again");
            navigate("/login", { replace: true });
        } catch (e) {
            toast.error(`change password: ${humanizeError(e)}`);
        }
    }

    return (
        <PageShell
            title="Account"
            subtitle={user ? `Signed in as ${user.username}` : "Account"}
            bodyPadding={8}
        >
            <div style={{ maxWidth: 720 }}>
                <Tabs defaultValue="identity">
                    <TabsList data-testid="account-tabs" className="mb-4">
                        <TabsTrigger value="identity">Identity</TabsTrigger>
                        <TabsTrigger value="password">Password</TabsTrigger>
                        <TabsTrigger value="tokens">Tokens</TabsTrigger>
                    </TabsList>

                    <TabsContent value="identity" className="space-y-4">
                        {user && (
                            <Card header="Identity" padding={5}>
                                <DataList
                                    items={[
                                        { label: "username", value: <Mono>{user.username}</Mono> },
                                        { label: "role", value: user.role },
                                        { label: "id", value: <Mono size={11}>{user.id}</Mono> },
                                    ]}
                                />
                            </Card>
                        )}
                    </TabsContent>

                    <TabsContent value="password" className="space-y-4">
                        <Card header="Change password" padding={5}>
                            <p
                                style={{
                                    color: palette.textSecondary,
                                    fontSize: 13,
                                    lineHeight: 1.5,
                                    marginTop: 0,
                                    marginBottom: space[4],
                                }}
                            >
                                Updating your password signs you out everywhere — all
                                existing sessions on this and other devices end.
                            </p>
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
                                    <div className="flex justify-end">
                                        <Button
                                            type="submit"
                                            disabled={pwForm.formState.isSubmitting}
                                        >
                                            {pwForm.formState.isSubmitting && (
                                                <Loader2 className="size-3.5 animate-spin" />
                                            )}
                                            Update password
                                        </Button>
                                    </div>
                                </form>
                            </Form>
                        </Card>
                    </TabsContent>

                    <TabsContent value="tokens" className="space-y-4">
                        <AccountTokensTab />
                    </TabsContent>
                </Tabs>
            </div>
        </PageShell>
    );
}
