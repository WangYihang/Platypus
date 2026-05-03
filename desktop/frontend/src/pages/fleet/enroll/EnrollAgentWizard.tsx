import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { useCurrentProject } from "../../../layout/ProjectShell";
import {
    IssueInstallResponse,
    getServerInfo,
    issueInstallArtifact,
    listInstallPlatforms,
} from "../../../lib/api";
import { humanizeError } from "../../../lib/humanizeError";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";

import {
    ARCH_ORDER,
    OS_ORDER,
    archLabel,
    osLabel,
    preferredOrder,
} from "../../enrollment/platforms";
import StepIndicator from "./StepIndicator";
import WizardFooter from "./WizardFooter";
import OSStep from "./steps/OSStep";
import ArchStep from "./steps/ArchStep";
import ServerEndpointStep from "./steps/ServerEndpointStep";
import DownloadTLSStep from "./steps/DownloadTLSStep";
import TTLStep from "./steps/TTLStep";
import PATMaxUsesStep from "./steps/PATMaxUsesStep";
import AutoApproveStep from "./steps/AutoApproveStep";
import BaselinePluginsStep from "./steps/BaselinePluginsStep";
import DescriptionStep from "./steps/DescriptionStep";
import ReviewStep from "./steps/ReviewStep";
import RunStep from "./steps/RunStep";
import { PlatformsState, STEPS, Step } from "./steps";
import { useEnrollWizardOpen } from "./useEnrollWizardOpen";

export default function EnrollAgentWizard() {
    const project = useCurrentProject();
    const { open, setOpen } = useEnrollWizardOpen();

    const [step, setStep] = useState<Step>("server");
    const [skipTLSVerification, setSkipTLSVerification] = useState(true);
    const [targetOS, setTargetOS] = useState("");
    const [targetArch, setTargetArch] = useState("");
    const [serverEndpoint, setServerEndpoint] = useState("");
    const [ttlSeconds, setTtlSeconds] = useState<number | undefined>(undefined);
    const [patMaxUses, setPatMaxUses] = useState<number | undefined>(undefined);
    const [autoApprove, setAutoApprove] = useState(false);
    const [baselinePluginIDs, setBaselinePluginIDs] = useState<string[]>([]);
    const [description, setDescription] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [issued, setIssued] = useState<IssueInstallResponse | null>(null);
    const [platforms, setPlatforms] = useState<PlatformsState>({ status: "loading" });

    // Reset on every open transition. Remounting on close-then-open
    // would flash the platforms-loading skeleton each time, so we
    // keep the wizard mounted and snap state back instead.
    useEffect(() => {
        if (!open) return;
        setStep("server");
        setTargetOS("");
        setTargetArch("");
        setTtlSeconds(undefined);
        setPatMaxUses(undefined);
        setAutoApprove(false);
        setBaselinePluginIDs([]);
        setDescription("");
        setSkipTLSVerification(true);
        setIssued(null);
        setSubmitting(false);

        // Best-effort prefill. Failures fall through to a blank input
        // — the operator just types it in step 3.
        getServerInfo()
            .then((info) => {
                if (info.public_addr) setServerEndpoint(info.public_addr);
            })
            .catch(() => {
                /* leave blank */
            });

        setPlatforms({ status: "loading" });
        listInstallPlatforms()
            .then((r) => {
                if (r.platforms.length === 0) {
                    setPlatforms({ status: "empty", channel: r.channel });
                } else {
                    setPlatforms({
                        status: "ready",
                        platforms: r.platforms,
                        channel: r.channel,
                    });
                }
            })
            .catch((e) => {
                setPlatforms({ status: "error", message: humanizeError(e) });
            });
    }, [open]);

    // Index the flat platform list by OS so step 2 can show only
    // architectures the active channel actually publishes for the OS
    // step 1 picked.
    const archsByOS = useMemo(() => {
        const map = new Map<string, string[]>();
        if (platforms.status !== "ready") return map;
        const tmp = new Map<string, Set<string>>();
        for (const p of platforms.platforms) {
            if (!tmp.has(p.os)) tmp.set(p.os, new Set());
            tmp.get(p.os)!.add(p.arch);
        }
        for (const [os, set] of tmp) {
            map.set(os, [...set].sort(preferredOrder(ARCH_ORDER)));
        }
        return map;
    }, [platforms]);
    const osList = useMemo(
        () => [...archsByOS.keys()].sort(preferredOrder(OS_ORDER)),
        [archsByOS],
    );
    const archList = targetOS ? (archsByOS.get(targetOS) ?? []) : [];

    async function generateCommands() {
        if (!serverEndpoint.trim()) return;
        setSubmitting(true);
        try {
            const r = await issueInstallArtifact(project.id, {
                server_endpoint: serverEndpoint.trim(),
                target_os: targetOS || undefined,
                target_arch: targetArch || undefined,
                ttl_seconds: ttlSeconds,
                pat_max_uses: patMaxUses,
                auto_approve: autoApprove,
                pat_description: description.trim() || undefined,
                baseline_plugin_ids:
                    baselinePluginIDs.length > 0 ? baselinePluginIDs : undefined,
            });
            setIssued(r);
            setStep("run");
        } catch (e) {
            toast.error(`Couldn't generate install command: ${humanizeError(e)}`);
        } finally {
            setSubmitting(false);
        }
    }

    function finishToFleet() {
        // Strip ?enroll AND set ?await=enroll in one history entry so
        // the existing wait banner picks up the new agent without the
        // URL flickering through an intermediate state.
        setOpen(false, { key: "await", value: "enroll" });
    }

    const currentIdx = STEPS.indexOf(step);
    const isFirst = currentIdx <= 0;
    const isLastBeforeRun = step === "review";
    const canNext =
        (step !== "server" || !!serverEndpoint.trim()) &&
        (step !== "pat_max_uses" ||
            patMaxUses === undefined ||
            (Number.isFinite(patMaxUses) && patMaxUses > 0)) &&
        (step !== "ttl" ||
            ttlSeconds === undefined ||
            (Number.isFinite(ttlSeconds) && ttlSeconds > 0));

    function goNext() {
        if (!canNext || isLastBeforeRun) return;
        const next = STEPS[currentIdx + 1];
        if (next && next !== "run") setStep(next);
    }

    function goBack() {
        if (isFirst) return;
        const prev = STEPS[currentIdx - 1];
        if (prev) setStep(prev);
    }

    const reviewSummary: Array<{ label: string; value: string; editStep: Step }> = [
        {
            label: "Server endpoint",
            value: serverEndpoint.trim() || "(required)",
            editStep: "server",
        },
        {
            label: "Download TLS",
            value: skipTLSVerification ? "Skip verification" : "Strict verification",
            editStep: "download_tls",
        },
        {
            label: "Target OS",
            value: targetOS ? osLabel(targetOS) : "Auto-detect",
            editStep: "os",
        },
        {
            label: "Target Arch",
            value: targetArch ? archLabel(targetArch) : "Auto-detect",
            editStep: "arch",
        },
        {
            label: "Install URL TTL",
            value: ttlSeconds ? `${ttlSeconds}s` : "Default (300s)",
            editStep: "ttl",
        },
        {
            label: "PAT max uses",
            value: patMaxUses ? String(patMaxUses) : "Default (1)",
            editStep: "pat_max_uses",
        },
        {
            label: "Approval policy",
            value: autoApprove ? "Auto-approve" : "Manual approval",
            editStep: "auto_approve",
        },
        {
            label: "Baseline plugins",
            value:
                baselinePluginIDs.length === 0
                    ? "(none — agent boots empty)"
                    : `${baselinePluginIDs.length}: ${baselinePluginIDs.join(", ")}`,
            editStep: "baseline_plugins",
        },
        {
            label: "Description",
            value: description.trim() || "(none)",
            editStep: "description",
        },
    ];

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogContent
                className="sm:max-w-[640px]"
                data-testid="enroll-wizard"
            >
                <DialogHeader>
                    <DialogTitle>Enroll agent</DialogTitle>
                    <DialogDescription>
                        Generate a one-shot install command for a new host. The agent
                        appears in Fleet automatically once it dials back.
                    </DialogDescription>
                </DialogHeader>

                <StepIndicator current={step} />

                {step === "server" && (
                    <ServerEndpointStep
                        serverEndpoint={serverEndpoint}
                        onChange={setServerEndpoint}
                    />
                )}
                {step === "download_tls" && (
                    <DownloadTLSStep
                        skipTLS={skipTLSVerification}
                        onSkipTLSChange={setSkipTLSVerification}
                    />
                )}
                {step === "os" && (
                    <OSStep
                        platforms={platforms}
                        osList={osList}
                        value={targetOS}
                        onChange={(v) => {
                            setTargetOS(v);
                            // Switching OS invalidates the prior arch
                            // pick — the same arch name on a
                            // different OS is still a re-pick, not a
                            // carry-over, otherwise the cascade can
                            // drift out of sync silently.
                            setTargetArch("");
                        }}
                        onPickPreset={(preset) => {
                            // Quick-pick: lock both OS and arch and
                            // skip past the arch step. The operator
                            // can still go Back if they want a
                            // different arch.
                            setTargetOS(preset.os);
                            setTargetArch(preset.arch);
                            setStep("ttl");
                        }}
                    />
                )}
                {step === "arch" && (
                    <ArchStep
                        os={targetOS}
                        archList={archList}
                        value={targetArch}
                        onChange={setTargetArch}
                    />
                )}
                {step === "ttl" && (
                    <TTLStep ttlSeconds={ttlSeconds} onChange={setTtlSeconds} />
                )}
                {step === "pat_max_uses" && (
                    <PATMaxUsesStep
                        patMaxUses={patMaxUses}
                        onChange={setPatMaxUses}
                    />
                )}
                {step === "auto_approve" && (
                    <AutoApproveStep
                        autoApprove={autoApprove}
                        onChange={setAutoApprove}
                    />
                )}
                {step === "baseline_plugins" && (
                    <BaselinePluginsStep
                        selected={baselinePluginIDs}
                        onChange={setBaselinePluginIDs}
                    />
                )}
                {step === "description" && (
                    <DescriptionStep
                        description={description}
                        onChange={setDescription}
                    />
                )}
                {step === "review" && (
                    <ReviewStep summary={reviewSummary} onEdit={setStep} />
                )}
                {step === "run" && issued && (
                    <RunStep
                        result={issued}
                        initialSkipTLS={skipTLSVerification}
                    />
                )}

                <WizardFooter
                    step={step}
                    submitting={submitting}
                    canNext={canNext}
                    canGenerate={!!serverEndpoint.trim()}
                    isFirst={isFirst}
                    onBack={goBack}
                    onNext={goNext}
                    onGenerate={generateCommands}
                    onCancel={() => setOpen(false)}
                    onFinish={finishToFleet}
                />
            </DialogContent>
        </Dialog>
    );
}
