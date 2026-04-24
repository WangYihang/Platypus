import { useState } from "react";
import { ArrowLeft, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNavigate } from "react-router-dom";

import Card from "../components/Card";
import { bootstrap, login } from "../lib/auth";
import { getServer, listServers } from "../lib/servers";
import { font, palette, space } from "../layout/theme";

import { Button } from "@/components/ui/button";
import {
    Form,
    FormControl,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const loginSchema = z.object({
    url: z.string().url("Must be a valid URL"),
    username: z.string().min(1, "Required"),
    password: z.string().min(1, "Required"),
});
type LoginFormValues = z.infer<typeof loginSchema>;

const bootstrapSchema = z.object({
    url: z.string().url("Must be a valid URL"),
    secret: z.string().min(1, "Required"),
    username: z.string().min(1, "Required"),
    password: z.string().min(8, "Min 8 chars"),
});
type BootstrapFormValues = z.infer<typeof bootstrapSchema>;

interface Props {
    onLoggedIn: () => void;
    initialURL?: string;
    // Pinned to a specific saved ServerProfile when the rail sent
    // the user here to re-auth (e.g. clicking a signed-out tile).
    // Hides the URL field, the First-time setup tab, and the
    // bootstrap flow.
    pinnedServerId?: string;
}

// Login is the auth gate for both web and desktop modes. Two flows in
// one card via a tab control: standard username+password login, and
// the one-shot bootstrap flow that creates the first admin from the
// API secret printed on server startup.
export default function Login({ onLoggedIn, initialURL, pinnedServerId }: Props) {
    const [busy, setBusy] = useState(false);
    const navigate = useNavigate();

    const pinnedProfile = pinnedServerId ? getServer(pinnedServerId) : null;
    const defaultURL =
        pinnedProfile?.url ||
        initialURL ||
        (typeof window !== "undefined"
            ? window.location.origin.replace(/:\d+$/, ":7331")
            : "");
    const hasSavedServers = listServers().length > 0;

    const loginForm = useForm<LoginFormValues>({
        resolver: zodResolver(loginSchema),
        defaultValues: { url: defaultURL, username: "", password: "" },
    });
    const bootstrapForm = useForm<BootstrapFormValues>({
        resolver: zodResolver(bootstrapSchema),
        defaultValues: { url: defaultURL, secret: "", username: "admin", password: "" },
    });

    async function doLogin(v: LoginFormValues) {
        setBusy(true);
        try {
            await login(pinnedProfile ?? v.url, v.username, v.password);
            onLoggedIn();
        } catch (err) {
            toast.error(`login: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    async function doBootstrap(v: BootstrapFormValues) {
        setBusy(true);
        try {
            await bootstrap(v.url, v.secret, v.username, v.password);
            toast.success("Admin created — welcome to Platypus");
            onLoggedIn();
        } catch (err) {
            toast.error(`bootstrap: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <div
            style={{
                minHeight: "100vh",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                padding: space[6],
                background: palette.main,
                color: palette.textPrimary,
            }}
        >
            <div style={{ width: 440, maxWidth: "100%" }}>
                <div style={{ marginBottom: space[6], textAlign: "left" }}>
                    {hasSavedServers && (
                        <button
                            onClick={() => navigate("/projects")}
                            style={{
                                display: "inline-flex",
                                alignItems: "center",
                                gap: 4,
                                background: "none",
                                border: "none",
                                color: palette.textMuted,
                                fontSize: 12,
                                cursor: "pointer",
                                padding: 0,
                                marginBottom: space[2],
                            }}
                        >
                            <ArrowLeft className="size-3" />
                            Back to servers
                        </button>
                    )}
                    <h1
                        style={{
                            margin: 0,
                            color: palette.textPrimary,
                            fontFamily: font.sans,
                            fontWeight: 600,
                            fontSize: 28,
                            lineHeight: 1.2,
                            letterSpacing: -0.2,
                        }}
                    >
                        {pinnedProfile ? pinnedProfile.name : "Platypus"}
                    </h1>
                    <p
                        style={{
                            margin: `${space[2]}px 0 0`,
                            color: palette.textSecondary,
                            fontSize: 14,
                            lineHeight: 1.5,
                        }}
                    >
                        {pinnedProfile
                            ? `Sign back in to ${pinnedProfile.url}.`
                            : "Log in to your server, or bootstrap the first admin from the startup secret."}
                    </p>
                </div>

                <Card padding={6}>
                    <Tabs defaultValue="login" className="w-full">
                        {!pinnedProfile && (
                            <TabsList className="mb-4 grid w-full grid-cols-2">
                                <TabsTrigger value="login">Log in</TabsTrigger>
                                <TabsTrigger value="bootstrap">First-time setup</TabsTrigger>
                            </TabsList>
                        )}

                        <TabsContent value="login">
                            <Form {...loginForm}>
                                <form
                                    onSubmit={loginForm.handleSubmit(doLogin)}
                                    className="space-y-4"
                                >
                                    {!pinnedProfile && (
                                        <FormField
                                            control={loginForm.control}
                                            name="url"
                                            render={({ field }) => (
                                                <FormItem>
                                                    <FormLabel>Server URL</FormLabel>
                                                    <FormControl>
                                                        <Input
                                                            placeholder="http://127.0.0.1:7331"
                                                            {...field}
                                                        />
                                                    </FormControl>
                                                    <FormMessage />
                                                </FormItem>
                                            )}
                                        />
                                    )}
                                    {pinnedProfile && (
                                        <div
                                            style={{
                                                fontFamily: font.mono,
                                                fontSize: 12,
                                                color: palette.textMuted,
                                                marginBottom: space[2],
                                            }}
                                        >
                                            {pinnedProfile.url}
                                        </div>
                                    )}
                                    <FormField
                                        control={loginForm.control}
                                        name="username"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Username</FormLabel>
                                                <FormControl>
                                                    <Input autoFocus placeholder="admin" {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <FormField
                                        control={loginForm.control}
                                        name="password"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Password</FormLabel>
                                                <FormControl>
                                                    <Input type="password" {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <Button type="submit" className="w-full" disabled={busy}>
                                        {busy && <Loader2 className="size-3.5 animate-spin" />}
                                        Log in
                                    </Button>
                                </form>
                            </Form>
                        </TabsContent>

                        <TabsContent value="bootstrap">
                            <p
                                style={{
                                    margin: `0 0 ${space[4]}px`,
                                    color: palette.textSecondary,
                                    fontSize: 13,
                                    lineHeight: 1.5,
                                }}
                            >
                                Use the{" "}
                                <span style={{ fontFamily: font.mono, fontSize: 12 }}>
                                    API bootstrap secret
                                </span>{" "}
                                printed on server startup. After the first admin exists this tab
                                stops working.
                            </p>
                            <Form {...bootstrapForm}>
                                <form
                                    onSubmit={bootstrapForm.handleSubmit(doBootstrap)}
                                    className="space-y-4"
                                >
                                    <FormField
                                        control={bootstrapForm.control}
                                        name="url"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Server URL</FormLabel>
                                                <FormControl>
                                                    <Input {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <FormField
                                        control={bootstrapForm.control}
                                        name="secret"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Server secret</FormLabel>
                                                <FormControl>
                                                    <Input type="password" {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <FormField
                                        control={bootstrapForm.control}
                                        name="username"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Admin username</FormLabel>
                                                <FormControl>
                                                    <Input {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <FormField
                                        control={bootstrapForm.control}
                                        name="password"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Admin password</FormLabel>
                                                <FormControl>
                                                    <Input type="password" {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <Button type="submit" className="w-full" disabled={busy}>
                                        {busy && <Loader2 className="size-3.5 animate-spin" />}
                                        Create admin
                                    </Button>
                                </form>
                            </Form>
                        </TabsContent>
                    </Tabs>
                </Card>
            </div>
        </div>
    );
}
