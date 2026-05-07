import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { useCurrentProject } from "../../../layout/ProjectShell";
import {
    EnrollmentPreset,
    IssueInstallResponse,
    PluginSpecRef,
    getServerInfo,
    issueInstallArtifact,
    listEnrollmentPresets,
    listInstallPlatforms,
    seedEnrollmentPresets,
} from "../../../lib/api";
import { PluginSpecDraft } from "../../../components/PluginSpecEditor";
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
import PickPresetStep, { PresetsState } from "./steps/PickPresetStep";
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

    const [step, setStep] = useState<Step>("pick_preset");
    const [skipTLSVerification, setSkipTLSVerification] = useState(true);
    const [targetOS, setTargetOS] = useState("");
    const [targetArch, setTargetArch] = useState("");
    const [serverEndpoint, setServerEndpoint] = useState("");
    const [liveServerEndpoint, setLiveServerEndpoint] = useState("");
    const [ttlSeconds, setTtlSeconds] = useState<number | undefined>(undefined);
    const [patMaxUses, setPatMaxUses] = useState<number | undefined>(undefined);
    const [autoApprove, setAutoApprove] = useState(false);
    // pluginSpecs is the rich operator-authored shape — plugin_id +
    // version + granted_capabilities + config_overrides +
    // schema_version per entry. Replaces the legacy []string of
    // plugin ids. The save-as-preset + issueInstallArtifact paths
    // each project to the right wire shape.
    const [pluginSpecs, setPluginSpecs] = useState<PluginSpecDraft[]>([]);
    const [description, setDescription] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [issued, setIssued] = useState<IssueInstallResponse | null>(null);
    const [platforms, setPlatforms] = useState<PlatformsState>({ status: "loading" });
    const [presets, setPresets] = useState<PresetsState>({ status: "loading" });
    // Tracks which preset (if any) the operator is currently editing.
    // Set when the user clicks "Edit" on a preset card and used by the
    // Save-as-preset action on Review to PUT instead of POST.
    const [editingPresetID, setEditingPresetID] = useState<string | null>(
        null,
    );

    // Reset on every open transition. Remounting on close-then-open
    // would flash the platforms-loading skeleton each time, so we
    // keep the wizard mounted and snap state back instead.
    useEffect(() => {
        if (!open) return;
        setStep("pick_preset");
        setTargetOS("");
        setTargetArch("");
        setTtlSeconds(undefined);
        setPatMaxUses(undefined);
        setAutoApprove(false);
        setPluginSpecs([]);
        setDescription("");
        setSkipTLSVerification(true);
        setIssued(null);
        setSubmitting(false);
        setEditingPresetID(null);

        // Best-effort prefill. Failures fall through to a blank input
        // — the operator just types it in step 3.
        getServerInfo()
            .then((info) => {
                if (info.public_addr) {
                    setServerEndpoint(info.public_addr);
                    setLiveServerEndpoint(info.public_addr);
                }
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

        setPresets({ status: "loading" });
        listEnrollmentPresets(project.id)
            .then(async (rows) => {
                // First open of a fresh project: the list comes back
                // empty, so seed three system defaults from the live
                // install manifest. Idempotent on the server (INSERT
                // OR IGNORE keyed on (project_id, name)) so this stays
                // safe even if a second client races us.
                if (rows.length === 0) {
                    try {
                        const seeded = await seedEnrollmentPresets(project.id);
                        setPresets({ status: "ready", presets: seeded });
                        return;
                    } catch {
                        // Seeding is opportunistic. If the BE refuses
                        // (no manifest, distributor disabled, …) we
                        // still want the picker to render its empty
                        // state instead of blocking on the seed step.
                        setPresets({ status: "ready", presets: [] });
                        return;
                    }
                }
                setPresets({ status: "ready", presets: rows });
            })
            .catch((e) => {
                setPresets({ status: "error", message: humanizeError(e) });
            });
    }, [open, project.id]);

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
                plugin_specs:
                    pluginSpecs.length > 0
                        ? (pluginSpecs as PluginSpecRef[])
                        : undefined,
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

    // specsFromPreset projects an EnrollmentPreset's plugin
    // selection into the wizard's PluginSpecDraft[] state. Rich
    // plugin_specs (PR 4 onward) wins when supplied — preserves
    // version + caps + config + schema_version. Falls back to
    // wrapping each id from the legacy baseline_plugin_ids field
    // for presets that haven't been re-saved since the migration.
    function specsFromPreset(p: EnrollmentPreset): PluginSpecDraft[] {
        if (p.plugin_specs && p.plugin_specs.length > 0) {
            return p.plugin_specs as PluginSpecDraft[];
        }
        const ids = p.baseline_plugin_ids ?? [];
        return ids.map((id) => ({ plugin_id: id }));
    }

    // applyPreset snaps every wizard field to the preset's saved
    // values and jumps the operator straight to Review. Server
    // endpoint falls back to the live server when the preset doesn't
    // carry one (older presets, or a deliberate "use whatever the
    // server is right now" choice). The picker shows a stale-server
    // warning if the saved endpoint diverges from the live one — by
    // the time we apply, the operator has already acknowledged it.
    function applyPreset(p: EnrollmentPreset) {
        setServerEndpoint(p.server_endpoint || liveServerEndpoint || "");
        setTargetOS(p.target_os || "");
        setTargetArch(p.target_arch || "");
        setTtlSeconds(p.ttl_seconds);
        setPatMaxUses(p.pat_max_uses);
        setAutoApprove(p.auto_approve);
        setSkipTLSVerification(p.skip_tls_verification);
        setPluginSpecs(specsFromPreset(p));
        setDescription(p.pat_description ?? "");
        setEditingPresetID(null);
        setStep("review");
    }

    // editPreset prefills wizard state from a preset and starts the
    // operator at the Server step. The Review step's "Save as preset"
    // PUTs (rather than POSTs) while editingPresetID is non-null.
    function editPreset(p: EnrollmentPreset) {
        setServerEndpoint(p.server_endpoint || liveServerEndpoint || "");
        setTargetOS(p.target_os || "");
        setTargetArch(p.target_arch || "");
        setTtlSeconds(p.ttl_seconds);
        setPatMaxUses(p.pat_max_uses);
        setAutoApprove(p.auto_approve);
        setSkipTLSVerification(p.skip_tls_verification);
        setPluginSpecs(specsFromPreset(p));
        setDescription(p.pat_description ?? "");
        setEditingPresetID(p.preset_id);
        setStep("server");
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
        // pick_preset doesn't have a Next — the picker carries its
        // own "Use" / "Start blank" affordances. Defensive guard so
        // the keyboard / external Next path can't silently advance
        // past it.
        if (step === "pick_preset") return;
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
                pluginSpecs.length === 0
                    ? "(none — agent boots empty)"
                    : `${pluginSpecs.length}: ${pluginSpecs.map((s) => s.plugin_id).join(", ")}`,
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

                {step === "pick_preset" && (
                    <PickPresetStep
                        projectID={project.id}
                        state={presets}
                        liveServerEndpoint={liveServerEndpoint}
                        onPick={applyPreset}
                        onStartBlank={() => {
                            setEditingPresetID(null);
                            setStep("server");
                        }}
                        onEdit={editPreset}
                        onDeleted={(id) => {
                            if (presets.status !== "ready") return;
                            setPresets({
                                status: "ready",
                                presets: presets.presets.filter(
                                    (p) => p.preset_id !== id,
                                ),
                            });
                        }}
                    />
                )}
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
                            // Auto-advance to the arch step on a
                            // confirmed pick. Empty `v` means the
                            // operator deselected (ToggleGroup single
                            // toggles off on a second click) — stay
                            // put so they can re-pick.
                            if (v) setStep("arch");
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
                        value={pluginSpecs}
                        onChange={setPluginSpecs}
                    />
                )}
                {step === "description" && (
                    <DescriptionStep
                        description={description}
                        onChange={setDescription}
                    />
                )}
                {step === "review" && (
                    <ReviewStep
                        summary={reviewSummary}
                        onEdit={setStep}
                        projectID={project.id}
                        editingPresetID={editingPresetID}
                        presetSnapshot={{
                            // The wizard doesn't carry a "preset name"
                            // field; the operator types one in the
                            // Save form. Pass an empty string here so
                            // the form starts blank.
                            name: "",
                            description: undefined,
                            server_endpoint:
                                serverEndpoint.trim() || undefined,
                            target_os: targetOS || undefined,
                            target_arch: targetArch || undefined,
                            ttl_seconds: ttlSeconds,
                            pat_max_uses: patMaxUses,
                            auto_approve: autoApprove,
                            skip_tls_verification: skipTLSVerification,
                            plugin_specs:
                                pluginSpecs.length > 0
                                    ? pluginSpecs
                                    : undefined,
                            pat_description:
                                description.trim() || undefined,
                        }}
                        onSaved={(id) => {
                            setEditingPresetID(id);
                            // Refresh the cached list so the next time
                            // the operator hits Back on the picker
                            // they see the just-saved row.
                            listEnrollmentPresets(project.id)
                                .then((rows) =>
                                    setPresets({
                                        status: "ready",
                                        presets: rows,
                                    }),
                                )
                                .catch(() => {
                                    /* leave stale; list refreshes on next open */
                                });
                        }}
                    />
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
                    onFinish={finishToFleet}
                />
            </DialogContent>
        </Dialog>
    );
}
