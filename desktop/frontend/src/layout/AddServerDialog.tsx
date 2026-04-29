import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { humanizeError } from "../lib/humanizeError";

import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { palette, space } from "./theme";
import {
    ServerProfile,
    addServer,
    defaultServerURL,
    hostnameFromURL,
    normaliseURL,
} from "../lib/servers";
import {
    PublicServerInfo,
    bootstrap,
    forgetAndRemoveServer,
    login,
    probeServer,
    switchServer,
} from "../lib/auth";
import { showAdminCreatedToast } from "../lib/bootstrapToast";

interface Props {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    onAdded?: (profile: ServerProfile) => void;
}

type Phase = "probe" | "login" | "bootstrap";

// AddServerDialog is the lightweight counterpart to the full-page
// onboarding wizard. Same flow (URL → probe → login or bootstrap),
// but rendered inside a dialog for users who already have one or
// more servers saved.
export default function AddServerDialog({ open, onOpenChange, onAdded }: Props) {
    const [phase, setPhase] = useState<Phase>("probe");
    const { t } = useTranslation("layout");
    const { t: to } = useTranslation("onboarding");
    const { t: tc } = useTranslation("common");

    // Shared fields across phases
    const [url, setUrl] = useState(defaultServerURL());
    const [name, setName] = useState("");
    const [probing, setProbing] = useState(false);
    const [probeInfo, setProbeInfo] = useState<PublicServerInfo | null>(null);
    const [probeError, setProbeError] = useState<string | null>(null);

    // Login
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");

    // Bootstrap
    const [secret, setSecret] = useState("");
    const [bootstrapUsername, setBootstrapUsername] = useState("admin");
    const [bootstrapPassword, setBootstrapPassword] = useState("");

    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (!open) return;
        setPhase("probe");
        setProbeInfo(null);
        setProbeError(null);
        setUsername("");
        setPassword("");
        setSecret("");
        setBootstrapPassword("");
        setBootstrapUsername("admin");
    }, [open]);

    const displayName = useMemo(
        () => name.trim() || (url ? hostnameFromURL(url) : ""),
        [name, url],
    );

    async function doProbe() {
        setProbing(true);
        setProbeInfo(null);
        setProbeError(null);
        try {
            const info = await probeServer(url);
            setProbeInfo(info);
            setPhase(info.admin_bootstrapped ? "login" : "bootstrap");
        } catch (err) {
            setProbeError(humanizeError(err));
        } finally {
            setProbing(false);
        }
    }

    async function finish(profile: ServerProfile) {
        onAdded?.(profile);
        await switchServer(profile.id);
        toast.success(`Connected to ${profile.name}`);
        onOpenChange(false);
    }

    async function doLogin() {
        setBusy(true);
        const profile = addServer({ name: displayName, url });
        try {
            await login(profile, username, password);
            await finish(profile);
        } catch (err) {
            toast.error(`login: ${humanizeError(err)}`);
            forgetAndRemoveServer(profile.id);
        } finally {
            setBusy(false);
        }
    }

    async function doBootstrap() {
        setBusy(true);
        const profile = addServer({ name: displayName, url });
        try {
            await bootstrap(profile, secret, bootstrapUsername, bootstrapPassword);
            onAdded?.(profile);
            await switchServer(profile.id);
            showAdminCreatedToast();
            onOpenChange(false);
        } catch (err) {
            toast.error(`bootstrap: ${humanizeError(err)}`);
            forgetAndRemoveServer(profile.id);
        } finally {
            setBusy(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[460px]">
                <DialogHeader>
                    <DialogTitle>{t("addServer.title")}</DialogTitle>
                    <DialogDescription>{t("addServer.description")}</DialogDescription>
                </DialogHeader>

                {phase === "probe" && (
                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        <Field label={tc("labels.serverUrl")}>
                            <Input
                                data-testid="add-server-url"
                                value={url}
                                onChange={(e) => setUrl(e.target.value)}
                                placeholder="http://127.0.0.1:7331"
                                autoFocus
                            />
                        </Field>
                        <Field label={tc("labels.displayName")}>
                            <Input
                                data-testid="add-server-name"
                                value={name}
                                onChange={(e) => setName(e.target.value)}
                                placeholder={hostnameFromURL(url)}
                            />
                        </Field>
                        {probeError && (
                            <div
                                style={{
                                    display: "flex",
                                    alignItems: "center",
                                    gap: space[2],
                                    color: palette.danger,
                                    fontSize: 12,
                                }}
                            >
                                <XCircle className="size-3.5" />
                                <span>{probeError}</span>
                            </div>
                        )}
                    </div>
                )}

                {(phase === "login" || phase === "bootstrap") && probeInfo && (
                    <div
                        style={{
                            display: "flex",
                            alignItems: "center",
                            gap: space[2],
                            color: palette.info,
                            fontSize: 12,
                            marginBottom: space[2],
                        }}
                    >
                        <CheckCircle2 className="size-3.5" />
                        <span>
                            {t("addServer.connectedHint", {
                                url: normaliseURL(url),
                                state: probeInfo.admin_bootstrapped
                                    ? t("addServer.stateLogin")
                                    : t("addServer.stateBootstrap"),
                            })}
                        </span>
                    </div>
                )}

                {phase === "login" && (
                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        <Field label={tc("labels.username")}>
                            <Input
                                data-testid="add-server-username"
                                autoFocus
                                value={username}
                                onChange={(e) => setUsername(e.target.value)}
                            />
                        </Field>
                        <Field label={tc("labels.password")}>
                            <Input
                                data-testid="add-server-password"
                                type="password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                            />
                        </Field>
                    </div>
                )}

                {phase === "bootstrap" && (
                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        <Field label={to("bootstrap.secretLabel")}>
                            <Input
                                autoFocus
                                type="password"
                                value={secret}
                                onChange={(e) => setSecret(e.target.value)}
                            />
                        </Field>
                        <Field label={to("bootstrap.adminUsername")}>
                            <Input
                                value={bootstrapUsername}
                                onChange={(e) => setBootstrapUsername(e.target.value)}
                            />
                        </Field>
                        <Field label={to("bootstrap.adminPassword")}>
                            <Input
                                type="password"
                                value={bootstrapPassword}
                                onChange={(e) => setBootstrapPassword(e.target.value)}
                            />
                        </Field>
                    </div>
                )}

                <DialogFooter>
                    {(phase === "login" || phase === "bootstrap") && (
                        <Button variant="outline" onClick={() => setPhase("probe")}>
                            {tc("actions.back")}
                        </Button>
                    )}
                    <Button variant="outline" onClick={() => onOpenChange(false)}>
                        {tc("actions.cancel")}
                    </Button>
                    {phase === "probe" && (
                        <Button onClick={doProbe} disabled={!url || probing}>
                            {probing && <Loader2 className="size-3.5 animate-spin" />}
                            {t("addServer.submitProbe")}
                        </Button>
                    )}
                    {phase === "login" && (
                        <Button
                            onClick={doLogin}
                            disabled={busy || !username || !password}
                        >
                            {busy && <Loader2 className="size-3.5 animate-spin" />}
                            {t("addServer.submitLogin")}
                        </Button>
                    )}
                    {phase === "bootstrap" && (
                        <Button
                            onClick={doBootstrap}
                            disabled={
                                busy ||
                                !secret ||
                                !bootstrapUsername ||
                                !bootstrapPassword
                            }
                        >
                            {busy && <Loader2 className="size-3.5 animate-spin" />}
                            {t("addServer.submitBootstrap")}
                        </Button>
                    )}
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function Field({
    label,
    children,
}: {
    label: string;
    children: React.ReactNode;
}) {
    return (
        <div>
            <Label
                style={{
                    fontSize: 12,
                    color: palette.textSecondary,
                    marginBottom: 4,
                    display: "inline-block",
                }}
            >
                {label}
            </Label>
            {children}
        </div>
    );
}
