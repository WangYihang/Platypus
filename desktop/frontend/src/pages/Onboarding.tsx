import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import Card from "../components/Card";
import WizardCard from "../components/WizardCard";
import { palette, space } from "../layout/theme";
import { humanizeError } from "../lib/humanizeError";
import {
    ServerProfile,
    addServer,
    hostnameFromURL,
} from "../lib/servers";
import {
    PublicServerInfo,
    bootstrap,
    forgetAndRemoveServer,
    login,
    probeServer,
} from "../lib/auth";

import BootstrapStep from "./onboarding/BootstrapStep";
import LoginStep from "./onboarding/LoginStep";
import ProbeStep from "./onboarding/ProbeStep";
import WelcomeStep from "./onboarding/WelcomeStep";
import { ProgressPill } from "./onboarding/common";

type Step = "welcome" | "probe" | "login" | "bootstrap";

// /onboarding — full-screen first-run wizard. Welcome → Your server
// (URL + probe) → Log in OR First-time setup (chosen from probe response).
// On success the user lands on /projects.
export default function Onboarding() {
    const navigate = useNavigate();
    const [step, setStep] = useState<Step>("welcome");

    const [url, setUrl] = useState("http://127.0.0.1:7331");
    const [name, setName] = useState("");
    const [probing, setProbing] = useState(false);
    const [probeInfo, setProbeInfo] = useState<PublicServerInfo | null>(null);
    const [probeError, setProbeError] = useState<string | null>(null);
    // URL the last failed probe was attempted with. Continue stays
    // disabled until the user edits the URL.
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

    // Reset login / bootstrap fields when returning to Step 2.
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

    // Editing the URL after a failed probe re-enables Continue.
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
            // probeServer throws a humanised ProbeError already.
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
            // Server cleans up <data-dir>/bootstrap.secret on the next
            // start once a user exists, but until then the operator can
            // delete it explicitly.
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
    const stepNumber = step === "welcome" ? 1 : step === "probe" ? 2 : 3;

    return (
        <WizardCard width={480}>
            <ProgressPill step={stepNumber} total={totalSteps} />
            <Card padding={6}>
                {step === "welcome" && <WelcomeStep onNext={() => setStep("probe")} />}

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
        </WizardCard>
    );
}
