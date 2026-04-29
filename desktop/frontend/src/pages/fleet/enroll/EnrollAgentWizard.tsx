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
    preferredOrder,
} from "../../enrollment/platforms";
import StepIndicator from "./StepIndicator";
import WizardFooter from "./WizardFooter";
import OSStep from "./steps/OSStep";
import ArchStep from "./steps/ArchStep";
import ConnectStep from "./steps/ConnectStep";
import RunStep from "./steps/RunStep";
import { PlatformsState, Step } from "./steps";
import { useEnrollWizardOpen } from "./useEnrollWizardOpen";

// EnrollAgentWizard is the inline-in-Fleet replacement for the dense
// modal that used to live on the standalone /fleet/enroll page. It
// walks the operator through OS → Arch → Connect → Run; the per-step
// UI lives in ./steps/*, and this module only orchestrates state and
// the open/close wiring.
//
// Open / closed state is driven by the URL search param `?enroll=1`
// (see useEnrollWizardOpen). That keeps the entry points dumb (every
// Link/Button just sets the param) and lets deep-links land directly
// inside the wizard. On close, the param is stripped.
//
// After step 4, "Done — show me Fleet" closes the wizard and
// switches the URL to `?await=enroll`, which arms the existing
// EnrollmentWaitBanner so the operator gets a live "did the agent
// dial back?" affordance without a separate UI surface.
export default function EnrollAgentWizard() {
    const project = useCurrentProject();
    const { open, setOpen } = useEnrollWizardOpen();

    const [step, setStep] = useState<Step>("os");
    const [targetOS, setTargetOS] = useState("");
    const [targetArch, setTargetArch] = useState("");
    const [serverEndpoint, setServerEndpoint] = useState("");
    const [ttlSeconds, setTtlSeconds] = useState<number | undefined>(undefined);
    const [autoApprove, setAutoApprove] = useState(false);
    const [description, setDescription] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [issued, setIssued] = useState<IssueInstallResponse | null>(null);
    const [platforms, setPlatforms] = useState<PlatformsState>({ status: "loading" });

    // Reset on every open transition. Remounting on close-then-open
    // would flash the platforms-loading skeleton each time, so we
    // keep the wizard mounted and snap state back instead.
    useEffect(() => {
        if (!open) return;
        setStep("os");
        setTargetOS("");
        setTargetArch("");
        setTtlSeconds(undefined);
        setAutoApprove(false);
        setDescription("");
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

    async function submitConnect() {
        if (!serverEndpoint.trim()) return;
        setSubmitting(true);
        try {
            const r = await issueInstallArtifact(project.id, {
                server_endpoint: serverEndpoint.trim(),
                target_os: targetOS || undefined,
                target_arch: targetArch || undefined,
                ttl_seconds: ttlSeconds,
                auto_approve: autoApprove,
                pat_description: description.trim() || undefined,
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
                            setStep("connect");
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
                {step === "connect" && (
                    <ConnectStep
                        serverEndpoint={serverEndpoint}
                        onServerEndpointChange={setServerEndpoint}
                        ttlSeconds={ttlSeconds}
                        onTtlSecondsChange={setTtlSeconds}
                        autoApprove={autoApprove}
                        onAutoApproveChange={setAutoApprove}
                        description={description}
                        onDescriptionChange={setDescription}
                    />
                )}
                {step === "run" && issued && <RunStep result={issued} />}

                <WizardFooter
                    step={step}
                    submitting={submitting}
                    canSubmitConnect={!!serverEndpoint.trim()}
                    onBack={() => {
                        if (step === "arch") setStep("os");
                        else if (step === "connect") setStep("arch");
                    }}
                    onNext={() => {
                        if (step === "os") setStep("arch");
                        else if (step === "arch") setStep("connect");
                    }}
                    onSubmit={submitConnect}
                    onCancel={() => setOpen(false)}
                    onFinish={finishToFleet}
                />
            </DialogContent>
        </Dialog>
    );
}
