import { Loader2, Pencil, Plus, Trash2, AlertTriangle } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { palette, space } from "../../../../layout/theme";
import { Button } from "@/components/ui/button";

import {
    EnrollmentPreset,
    deleteEnrollmentPreset,
} from "../../../../lib/api";
import { archLabel, osLabel } from "../../../enrollment/platforms";
import { humanizeError } from "../../../../lib/humanizeError";

export type PresetsState =
    | { status: "loading" }
    | { status: "ready"; presets: EnrollmentPreset[] }
    | { status: "error"; message: string };

interface Props {
    projectID: string;
    state: PresetsState;
    liveServerEndpoint: string;
    onPick: (preset: EnrollmentPreset) => void;
    onStartBlank: () => void;
    onEdit: (preset: EnrollmentPreset) => void;
    onDeleted: (presetID: string) => void;
}

// PickPresetStep is the new landing screen for the Enroll Agent
// wizard. It shows the project's saved enrollment presets so repeat
// operators can collapse the 11-step flow to one click; "Start blank"
// keeps the legacy authoring path available unchanged.
//
// Edit-in-place isn't here — clicking Edit jumps the operator into the
// existing 11-step wizard with the preset's values prefilled, then the
// Review step's "Save as preset" updates the same row. That keeps the
// pick screen lean: cards + actions, not a second mini-form.
export default function PickPresetStep({
    projectID,
    state,
    liveServerEndpoint,
    onPick,
    onStartBlank,
    onEdit,
    onDeleted,
}: Props) {
    if (state.status === "loading") {
        return (
            <div
                data-testid="enroll-wizard-pick-preset"
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 8,
                    fontSize: 13,
                    color: palette.textMuted,
                    padding: space[4],
                }}
            >
                <Loader2 className="size-4 animate-spin" /> Loading presets…
            </div>
        );
    }
    if (state.status === "error") {
        return (
            <div
                data-testid="enroll-wizard-pick-preset"
                className="space-y-3"
            >
                <div style={{ fontSize: 13, color: palette.danger }}>
                    Couldn't load presets: {state.message}
                </div>
                <Button
                    variant="outline"
                    size="sm"
                    onClick={onStartBlank}
                    data-testid="enroll-wizard-start-blank"
                >
                    Start blank instead
                </Button>
            </div>
        );
    }
    return (
        <div
            data-testid="enroll-wizard-pick-preset"
            className="space-y-3"
        >
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Pick a saved preset to skip ahead to Review, or start blank to
                walk through every option.
            </div>
            {state.presets.length === 0 ? (
                <div
                    style={{
                        fontSize: 12,
                        color: palette.textMuted,
                        border: `1px dashed ${palette.border}`,
                        borderRadius: 6,
                        padding: space[3],
                    }}
                >
                    No saved presets in this project yet. Walk through the
                    wizard once and use "Save as preset" on the Review step to
                    capture your defaults.
                </div>
            ) : (
                <div
                    className="space-y-2"
                    role="list"
                    data-testid="enroll-wizard-preset-list"
                >
                    {state.presets.map((p) => (
                        <PresetCard
                            key={p.preset_id}
                            preset={p}
                            projectID={projectID}
                            liveServerEndpoint={liveServerEndpoint}
                            onPick={() => onPick(p)}
                            onEdit={() => onEdit(p)}
                            onDeleted={() => onDeleted(p.preset_id)}
                        />
                    ))}
                </div>
            )}
            <div style={{ display: "flex", justifyContent: "flex-end" }}>
                <Button
                    variant="ghost"
                    size="sm"
                    onClick={onStartBlank}
                    data-testid="enroll-wizard-start-blank"
                >
                    <Plus className="size-3.5" /> Start blank
                </Button>
            </div>
        </div>
    );
}

function PresetCard({
    preset,
    projectID,
    liveServerEndpoint,
    onPick,
    onEdit,
    onDeleted,
}: {
    preset: EnrollmentPreset;
    projectID: string;
    liveServerEndpoint: string;
    onPick: () => void;
    onEdit: () => void;
    onDeleted: () => void;
}) {
    const [deleting, setDeleting] = useState(false);
    const stale =
        !!preset.server_endpoint &&
        !!liveServerEndpoint &&
        preset.server_endpoint !== liveServerEndpoint;
    const platformLabel = preset.target_os
        ? `${osLabel(preset.target_os)}${preset.target_arch ? ` · ${archLabel(preset.target_arch)}` : ""}`
        : "Auto-detect";
    const policyBits: string[] = [];
    if (preset.ttl_seconds) policyBits.push(`${preset.ttl_seconds}s TTL`);
    if (preset.pat_max_uses) policyBits.push(`${preset.pat_max_uses}× use`);
    policyBits.push(preset.auto_approve ? "auto-approve" : "manual approval");
    if (preset.plugin_specs && preset.plugin_specs.length > 0) {
        policyBits.push(`${preset.plugin_specs.length} plugin(s)`);
    }
    return (
        <div
            role="listitem"
            data-testid={`enroll-wizard-preset-${preset.preset_id}`}
            className="rounded border border-border bg-surface"
            style={{ padding: space[3] }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "flex-start",
                    justifyContent: "space-between",
                    gap: space[2],
                }}
            >
                <div style={{ minWidth: 0, flex: 1 }}>
                    <div
                        style={{
                            display: "flex",
                            alignItems: "baseline",
                            gap: 6,
                            fontSize: 13,
                            color: palette.textPrimary,
                            fontWeight: 600,
                        }}
                    >
                        <span>{preset.name}</span>
                        {preset.is_seed && (
                            <span
                                style={{
                                    fontSize: 10,
                                    color: palette.textMuted,
                                    fontWeight: 500,
                                    border: `1px solid ${palette.border}`,
                                    borderRadius: 3,
                                    padding: "0 4px",
                                }}
                            >
                                system
                            </span>
                        )}
                    </div>
                    <div
                        style={{
                            fontSize: 11,
                            color: palette.textMuted,
                            marginTop: 2,
                        }}
                    >
                        {platformLabel} · {policyBits.join(" · ")}
                    </div>
                    {preset.description && (
                        <div
                            style={{
                                fontSize: 11,
                                color: palette.textSecondary,
                                marginTop: 2,
                            }}
                        >
                            {preset.description}
                        </div>
                    )}
                    {stale && (
                        <div
                            style={{
                                fontSize: 11,
                                color: palette.warning,
                                marginTop: 4,
                                display: "inline-flex",
                                alignItems: "center",
                                gap: 4,
                            }}
                        >
                            <AlertTriangle className="size-3" />
                            Saved server endpoint differs from current
                            ({preset.server_endpoint} → {liveServerEndpoint}).
                            Picking will use the live one.
                        </div>
                    )}
                </div>
                <div style={{ display: "flex", gap: space[1], flexShrink: 0 }}>
                    <Button
                        size="sm"
                        onClick={onPick}
                        data-testid={`enroll-wizard-preset-pick-${preset.preset_id}`}
                    >
                        Use
                    </Button>
                    <Button
                        variant="ghost"
                        size="sm"
                        onClick={onEdit}
                        title="Edit in wizard"
                        data-testid={`enroll-wizard-preset-edit-${preset.preset_id}`}
                    >
                        <Pencil className="size-3.5" />
                    </Button>
                    <Button
                        variant="ghost"
                        size="sm"
                        disabled={deleting}
                        onClick={async () => {
                            setDeleting(true);
                            try {
                                await deleteEnrollmentPreset(
                                    projectID,
                                    preset.preset_id,
                                );
                                onDeleted();
                            } catch (e) {
                                toast.error(
                                    `Couldn't delete preset: ${humanizeError(e)}`,
                                );
                            } finally {
                                setDeleting(false);
                            }
                        }}
                        title="Delete preset"
                        data-testid={`enroll-wizard-preset-delete-${preset.preset_id}`}
                    >
                        {deleting ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <Trash2 className="size-3.5" />
                        )}
                    </Button>
                </div>
            </div>
        </div>
    );
}
