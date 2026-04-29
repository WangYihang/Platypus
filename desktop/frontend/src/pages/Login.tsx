import { useState } from "react";
import { ArrowLeft, Loader2 } from "lucide-react";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";
import { humanizeError } from "../lib/humanizeError";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNavigate } from "react-router-dom";

import Card from "../components/Card";
import Mono from "../components/Mono";
import WizardCard from "../components/WizardCard";
import { bootstrap, login } from "../lib/auth";
import { showAdminCreatedToast } from "../lib/bootstrapToast";
import { defaultServerURL, getServer, listServers } from "../lib/servers";
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
    const { t } = useTranslation("login");
    const { t: tc } = useTranslation("common");

    const pinnedProfile = pinnedServerId ? getServer(pinnedServerId) : null;
    // Default to the page's own origin so the embedded server bundle
    // (platypus-server serves the UI at /) just works on first visit.
    // Standalone-preview flows (`make web-ui-serve` on :7777 talking to a
    // server on :9443) need the user to overwrite this once; the value
    // persists in the saved server profile after that.
    const defaultURL = pinnedProfile?.url || initialURL || defaultServerURL();
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
            toast.error(`login: ${humanizeError(err)}`);
        } finally {
            setBusy(false);
        }
    }

    async function doBootstrap(v: BootstrapFormValues) {
        setBusy(true);
        try {
            await bootstrap(pinnedProfile ?? v.url, v.secret, v.username, v.password);
            showAdminCreatedToast();
            onLoggedIn();
        } catch (err) {
            toast.error(`bootstrap: ${humanizeError(err)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <WizardCard width={440}>
                <div style={{ marginBottom: space[6], textAlign: "left" }}>
                    {hasSavedServers && (
                        <button
                            onClick={() =>
                                // /projects is gated by RequireAuth, so without a
                                // session it just bounces back here pinned to the
                                // active server. Route through /onboarding instead
                                // — that page is unauthenticated and lets the user
                                // probe a different URL or finish first-time setup.
                                navigate(pinnedProfile ? "/onboarding" : "/projects")
                            }
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
                            {t("backToServers")}
                        </button>
                    )}
                    <h1
                        style={{
                            margin: 0,
                            color: palette.textPrimary,
                            fontFamily: font.mono,
                            fontWeight: 600,
                            fontSize: 28,
                            lineHeight: 1.2,
                            letterSpacing: -0.2,
                        }}
                    >
                        {pinnedProfile ? pinnedProfile.name : t("title")}
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
                            ? t("subtitlePinned", { url: pinnedProfile.url })
                            : t("subtitleDefault")}
                    </p>
                </div>

                <Card padding={6}>
                    <Tabs defaultValue="login" className="w-full">
                        <TabsList className="mb-4 grid w-full grid-cols-2">
                            <TabsTrigger value="login">{t("tabs.login")}</TabsTrigger>
                            <TabsTrigger value="bootstrap">{t("tabs.bootstrap")}</TabsTrigger>
                        </TabsList>

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
                                                    <FormLabel>{tc("labels.serverUrl")}</FormLabel>
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
                                                <FormLabel>{tc("labels.username")}</FormLabel>
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
                                                <FormLabel>{tc("labels.password")}</FormLabel>
                                                <FormControl>
                                                    <Input type="password" {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <Button type="submit" className="w-full" disabled={busy}>
                                        {busy && <Loader2 className="size-3.5 animate-spin" />}
                                        {tc("actions.logIn")}
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
                                <Trans
                                    ns="login"
                                    i18nKey="bootstrap.intro"
                                    components={{ mono: <Mono /> }}
                                />
                            </p>
                            <Form {...bootstrapForm}>
                                <form
                                    onSubmit={bootstrapForm.handleSubmit(doBootstrap)}
                                    className="space-y-4"
                                >
                                    {!pinnedProfile && (
                                        <FormField
                                            control={bootstrapForm.control}
                                            name="url"
                                            render={({ field }) => (
                                                <FormItem>
                                                    <FormLabel>{tc("labels.serverUrl")}</FormLabel>
                                                    <FormControl>
                                                        <Input {...field} />
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
                                        control={bootstrapForm.control}
                                        name="secret"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>{t("bootstrap.secretLabel")}</FormLabel>
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
                                                <FormLabel>{t("bootstrap.adminUsername")}</FormLabel>
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
                                                <FormLabel>{t("bootstrap.adminPassword")}</FormLabel>
                                                <FormControl>
                                                    <Input type="password" {...field} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <Button type="submit" className="w-full" disabled={busy}>
                                        {busy && <Loader2 className="size-3.5 animate-spin" />}
                                        {t("bootstrap.submit")}
                                    </Button>
                                </form>
                            </Form>
                        </TabsContent>
                    </Tabs>
                </Card>
        </WizardCard>
    );
}
