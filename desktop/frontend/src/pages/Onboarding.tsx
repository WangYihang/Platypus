import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowRight, CheckCircle2, Loader2, XCircle } from "lucide-react";
import { toast } from "sonner";
import { humanizeError } from "../lib/humanizeError";

import Card from "../components/Card";
import CopyButton from "../components/CopyButton";
import { palette, radius, space } from "../layout/theme";
import {
    ServerProfile,
    addServer,
    hostnameFromURL,
    normaliseURL,
} from "../lib/servers";
import {
    PublicServerInfo,
    bootstrap,
    forgetAndRemoveServer,
    login,
    probeServer,
} from "../lib/auth";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type Step = "welcome" | "probe" | "login" | "bootstrap";

// Onboarding is the full-screen first-run wizard. Lives at
// /onboarding and is the default destination when no servers are
// saved locally. Three progressive steps: Welcome → Your server
// (URL + probe) → Log in OR First-time setup (chosen from the probe
// response). On success we land the user on /projects.
export default function Onboarding() {
    const navigate = useNavigate();
    const [step, setStep] = useState<Step>("welcome");

    const [url, setUrl] = useState("http://127.0.0.1:7331");
    const [name, setName] = useState("");
    const [probing, setProbing] = useState(false);
    const [probeInfo, setProbeInfo] = useState<PublicServerInfo | null>(null);
    const [probeError, setProbeError] = useState<string | null>(null);
    // URL the last failed probe was attempted with. Continue stays
    // disabled until the user edits the URL — clicking again with the
    // same URL would just reproduce the same failure.
    const [lastFailedURL, setLastFailedURL] = useState<string | null>(null);

    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [secret, setSecret] = useState("");
    const [bootstrapUsername, setBootstrapUsername] = useState("admin");
    const [bootstrapPassword, setBootstrapPassword] = useState("");
    const [busy, setBusy] = useState(false);

    const displayName = useMemo(
        () => name.trim() || (url ? hostnameFromURL(url) : ""),
        [name, url],
    );

    // Reset the login / bootstrap fields every time the user returns
    // to Step 2 so typing a new URL starts clean.
    useEffect(() => {
        if (step === "probe") {
            setProbeInfo(null);
            setProbeError(null);
            setLastFailedURL(null);
            setUsername("");
            setPassword("");
            setSecret("");
            setBootstrapPassword("");
        }
    }, [step]);

    // Editing the URL after a failed probe clears the failed state so
    // the Continue button re-enables.
    useEffect(() => {
        if (lastFailedURL !== null && url !== lastFailedURL) {
            setProbeError(null);
            setLastFailedURL(null);
        }
    }, [url, lastFailedURL]);

    async function doProbe() {
        setProbing(true);
        setProbeInfo(null);
        setProbeError(null);
        try {
            const info = await probeServer(url);
            setProbeInfo(info);
            setLastFailedURL(null);
            setStep(info.admin_bootstrapped ? "login" : "bootstrap");
        } catch (err) {
            // probeServer throws ProbeError with a humanised message
            // already; render `.message` directly so the user doesn't
            // see "TypeError: …" and can act on the advice.
            const message = err instanceof Error ? err.message : String(err);
            setProbeError(message);
            setLastFailedURL(url);
        } finally {
            setProbing(false);
        }
    }

    async function finish(profile: ServerProfile) {
        toast.success(`Welcome to ${profile.name}`);
        navigate("/projects", { replace: true });
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
            // The server cleans up <data-dir>/bootstrap.secret
            // automatically on the next start once a user exists, but
            // until then the operator can delete it explicitly. Prompt
            // them — keeps the secret out of backups / volume
            // snapshots taken before the next restart.
            toast.info(
                "Admin created. Delete <data-dir>/bootstrap.secret on the server to keep the secret out of backups.",
                { duration: 8000 },
            );
            await finish(profile);
        } catch (err) {
            toast.error(`bootstrap: ${humanizeError(err)}`);
            forgetAndRemoveServer(profile.id);
        } finally {
            setBusy(false);
        }
    }

    const totalSteps = 3;
    const stepNumber =
        step === "welcome" ? 1 : step === "probe" ? 2 : 3;

    return (
        <div
            style={{
                minHeight: "100vh",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                background: palette.main,
                color: palette.textPrimary,
                padding: space[6],
            }}
        >
            <div style={{ width: 480, maxWidth: "100%" }}>
                <ProgressPill step={stepNumber} total={totalSteps} />
                <Card padding={6}>
                    {step === "welcome" && (
                        <WelcomeStep onNext={() => setStep("probe")} />
                    )}

                    {step === "probe" && (
                        <ProbeStep
                            url={url}
                            name={name}
                            probing={probing}
                            probeError={probeError}
                            probeInfo={probeInfo}
                            blocked={lastFailedURL !== null && lastFailedURL === url}
                            onURL={setUrl}
                            onName={setName}
                            onBack={() => setStep("welcome")}
                            onNext={doProbe}
                        />
                    )}

                    {step === "login" && probeInfo && (
                        <LoginStep
                            url={url}
                            username={username}
                            password={password}
                            busy={busy}
                            onUsername={setUsername}
                            onPassword={setPassword}
                            onBack={() => setStep("probe")}
                            onSubmit={doLogin}
                        />
                    )}

                    {step === "bootstrap" && probeInfo && (
                        <BootstrapStep
                            url={url}
                            secret={secret}
                            username={bootstrapUsername}
                            password={bootstrapPassword}
                            busy={busy}
                            onSecret={setSecret}
                            onUsername={setBootstrapUsername}
                            onPassword={setBootstrapPassword}
                            onBack={() => setStep("probe")}
                            onSubmit={doBootstrap}
                        />
                    )}
                </Card>

                <div
                    style={{
                        marginTop: space[4],
                        textAlign: "center",
                        color: palette.textMuted,
                        fontSize: 12,
                    }}
                >
                    <button
                        onClick={() => navigate("/login")}
                        style={{
                            background: "none",
                            border: "none",
                            color: palette.textMuted,
                            fontSize: 12,
                            cursor: "pointer",
                            padding: 0,
                            textDecoration: "underline",
                            textDecorationStyle: "dotted",
                        }}
                    >
                        Use the classic login form
                    </button>
                </div>
            </div>
        </div>
    );
}

// ----- step components ----------------------------------------------

function WelcomeStep({ onNext }: { onNext: () => void }) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <div>
                <h1
                    style={{
                        margin: 0,
                        fontSize: 28,
                        fontWeight: 600,
                        letterSpacing: -0.3,
                    }}
                >
                    Welcome to Platypus
                </h1>
                <p
                    style={{
                        margin: `${space[2]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 14,
                        lineHeight: 1.6,
                    }}
                >
                    Let's connect you to a server. It takes about thirty seconds.
                    You'll need the server's URL, and either a username/password
                    or, on a fresh install, the first-admin secret from
                    <code> &lt;data-dir&gt;/bootstrap.secret</code> on the server.
                </p>
            </div>
            <Button onClick={onNext} size="lg" data-testid="onboarding-get-started">
                Get started
                <ArrowRight className="size-4" />
            </Button>
        </div>
    );
}

interface ProbeStepProps {
    url: string;
    name: string;
    probing: boolean;
    probeError: string | null;
    probeInfo: PublicServerInfo | null;
    // True when the last probe attempt failed against this exact URL.
    // Continue stays disabled until the user edits the URL.
    blocked: boolean;
    onURL: (v: string) => void;
    onName: (v: string) => void;
    onBack: () => void;
    onNext: () => void;
}

function ProbeStep({
    url,
    name,
    probing,
    probeError,
    blocked,
    onURL,
    onName,
    onBack,
    onNext,
}: ProbeStepProps) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <div>
                <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>Your server</h2>
                <p
                    style={{
                        margin: `${space[1]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 13,
                    }}
                >
                    Paste the server URL. We'll probe it and pick the right next
                    step automatically.
                </p>
            </div>
            <Field label="Server URL">
                <Input
                    value={url}
                    onChange={(e) => onURL(e.target.value)}
                    placeholder="https://localhost:9443"
                    autoFocus
                    data-testid="onboarding-url"
                />
            </Field>
            <Field label="Display name (optional)">
                <Input
                    value={name}
                    onChange={(e) => onName(e.target.value)}
                    placeholder={hostnameFromURL(url)}
                    data-testid="onboarding-name"
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
            <div style={{ display: "flex", gap: space[2] }}>
                <Button variant="outline" onClick={onBack}>
                    Back
                </Button>
                <Button
                    onClick={onNext}
                    disabled={!url || probing || blocked}
                    style={{ marginLeft: "auto" }}
                    data-testid="onboarding-probe"
                >
                    {probing && <Loader2 className="size-3.5 animate-spin" />}
                    Continue
                </Button>
            </div>
        </div>
    );
}

interface LoginStepProps {
    url: string;
    username: string;
    password: string;
    busy: boolean;
    onUsername: (v: string) => void;
    onPassword: (v: string) => void;
    onBack: () => void;
    onSubmit: () => void;
}

function LoginStep({
    url,
    username,
    password,
    busy,
    onUsername,
    onPassword,
    onBack,
    onSubmit,
}: LoginStepProps) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <ConfirmedBanner
                url={url}
                hint="Ready to log in"
            />
            <div>
                <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>Log in</h2>
                <p
                    style={{
                        margin: `${space[1]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 13,
                    }}
                >
                    Use your Platypus credentials for this server.
                </p>
            </div>
            <Field label="Username">
                <Input
                    autoFocus
                    value={username}
                    onChange={(e) => onUsername(e.target.value)}
                    data-testid="onboarding-username"
                />
            </Field>
            <Field label="Password">
                <Input
                    type="password"
                    value={password}
                    onChange={(e) => onPassword(e.target.value)}
                    data-testid="onboarding-password"
                />
            </Field>
            <div style={{ display: "flex", gap: space[2] }}>
                <Button variant="outline" onClick={onBack}>
                    Back
                </Button>
                <Button
                    onClick={onSubmit}
                    disabled={busy || !username || !password}
                    style={{ marginLeft: "auto" }}
                    data-testid="onboarding-login"
                >
                    {busy && <Loader2 className="size-3.5 animate-spin" />}
                    Log in
                </Button>
            </div>
        </div>
    );
}

interface BootstrapStepProps {
    url: string;
    secret: string;
    username: string;
    password: string;
    busy: boolean;
    onSecret: (v: string) => void;
    onUsername: (v: string) => void;
    onPassword: (v: string) => void;
    onBack: () => void;
    onSubmit: () => void;
}

function BootstrapStep({
    url,
    secret,
    username,
    password,
    busy,
    onSecret,
    onUsername,
    onPassword,
    onBack,
    onSubmit,
}: BootstrapStepProps) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <ConfirmedBanner
                url={url}
                hint="Bootstrap admin"
            />
            <div>
                <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>
                    Create the first admin
                </h2>
                <p
                    style={{
                        margin: `${space[1]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 13,
                    }}
                >
                    Paste the first-admin secret from the server and pick admin credentials.
                </p>
            </div>
            <Field
                label="Server secret"
                helper={
                    <>
                        Look for <code>bootstrap.secret</code> in the server's data
                        directory (mode 0600, written on first boot). On Docker compose,
                        run the command below — copy it to clipboard with the button on
                        the right. You can delete the file once the first admin is created.
                        <div
                            style={{
                                display: "flex",
                                alignItems: "center",
                                gap: space[2],
                                marginTop: space[2],
                            }}
                        >
                            <code
                                style={{
                                    flex: 1,
                                    minWidth: 0,
                                    overflow: "auto",
                                    whiteSpace: "nowrap",
                                    background: palette.surface,
                                    border: `1px solid ${palette.border}`,
                                    borderRadius: radius.sm,
                                    padding: `4px ${space[2]}px`,
                                    fontSize: 12,
                                }}
                            >
                                docker compose exec platypus-server cat /app/data/bootstrap.secret
                            </code>
                            <CopyButton
                                text="docker compose exec platypus-server cat /app/data/bootstrap.secret"
                                label=""
                                successMessage="Command copied"
                            />
                        </div>
                    </>
                }
            >
                <Input
                    autoFocus
                    type="password"
                    value={secret}
                    onChange={(e) => onSecret(e.target.value)}
                    data-testid="onboarding-secret"
                />
            </Field>
            <Field label="Admin username">
                <Input
                    value={username}
                    onChange={(e) => onUsername(e.target.value)}
                />
            </Field>
            <Field label="Admin password">
                <Input
                    type="password"
                    value={password}
                    onChange={(e) => onPassword(e.target.value)}
                />
            </Field>
            <div style={{ display: "flex", gap: space[2] }}>
                <Button variant="outline" onClick={onBack}>
                    Back
                </Button>
                <Button
                    onClick={onSubmit}
                    disabled={busy || !secret || !username || !password}
                    style={{ marginLeft: "auto" }}
                >
                    {busy && <Loader2 className="size-3.5 animate-spin" />}
                    Create admin
                </Button>
            </div>
        </div>
    );
}

function ConfirmedBanner({ url, hint }: { url: string; hint: string }) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                padding: space[3],
                background: palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                color: palette.textSecondary,
                fontSize: 12,
            }}
        >
            <CheckCircle2 className="size-4" style={{ color: palette.info }} />
            <div style={{ flex: 1, minWidth: 0 }}>
                <div
                    style={{
                        fontFamily: "var(--font-geist-mono)",
                        color: palette.textPrimary,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                >
                    {normaliseURL(url)}
                </div>
                <div>{hint}</div>
            </div>
        </div>
    );
}

function ProgressPill({ step, total }: { step: number; total: number }) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                justifyContent: "center",
                marginBottom: space[4],
                color: palette.textMuted,
                fontSize: 12,
                fontFamily: "var(--font-geist-mono)",
            }}
        >
            <span>
                Step {step} / {total}
            </span>
            <div style={{ display: "flex", gap: 4 }}>
                {Array.from({ length: total }).map((_, i) => (
                    <span
                        key={i}
                        style={{
                            width: 24,
                            height: 3,
                            borderRadius: 2,
                            background:
                                i < step ? palette.textPrimary : palette.border,
                        }}
                    />
                ))}
            </div>
        </div>
    );
}

function Field({
    label,
    helper,
    children,
}: {
    label: string;
    // Optional one-line / one-paragraph guidance rendered below the
    // input. Used by the bootstrap-secret field to point operators at
    // <data-dir>/bootstrap.secret since the secret no longer appears
    // in stdout (see security audit M2).
    helper?: React.ReactNode;
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
            {helper && (
                <div
                    style={{
                        marginTop: 6,
                        fontSize: 12,
                        color: palette.textMuted,
                        lineHeight: 1.5,
                    }}
                >
                    {helper}
                </div>
            )}
        </div>
    );
}
